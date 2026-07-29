package api_test

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jacobrluttrull/gator/internal/auth"
)

func TestLoginWithCorrectPasswordReturnsAPIKey(t *testing.T) {
	h, db := testHandler(t)

	reg := doJSON(t, h, "POST", "/v1/register", `{"name": "alice", "password": "s3cret-pw"}`)
	if reg.Code != http.StatusCreated {
		t.Fatalf("register status = %d, want %d", reg.Code, http.StatusCreated)
	}

	rr := doJSON(t, h, "POST", "/v1/login", `{"name": "alice", "password": "s3cret-pw"}`)

	if rr.Code != http.StatusOK {
		t.Fatalf("login status = %d, want %d; body: %s", rr.Code, http.StatusOK, rr.Body.String())
	}
	var got struct {
		APIKey string `json:"api_key"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("response is not valid JSON: %v; body: %s", err, rr.Body.String())
	}
	if raw, err := hex.DecodeString(got.APIKey); err != nil || len(raw) != 32 {
		t.Fatalf("api_key = %q, want 32 random bytes hex-encoded", got.APIKey)
	}
	if strings.Contains(strings.ToLower(rr.Body.String()), "s3cret-pw") {
		t.Errorf("response leaks password material: %s", rr.Body.String())
	}

	// Story 26: a DB leak must not leak usable keys — the stored form is
	// the SHA-256 hash, never the key itself. Until the auth middleware
	// ticket lands there is no HTTP-observable way to check this.
	var stored int
	if err := db.QueryRowContext(context.Background(), "SELECT count(*) FROM api_keys WHERE key_hash = $1", got.APIKey).Scan(&stored); err != nil {
		t.Fatalf("querying api_keys for raw key: %v", err)
	}
	if stored != 0 {
		t.Errorf("raw API key found in api_keys.key_hash; keys must be stored hashed")
	}
	if err := db.QueryRowContext(context.Background(), "SELECT count(*) FROM api_keys WHERE key_hash = $1", auth.HashAPIKey(got.APIKey)).Scan(&stored); err != nil {
		t.Fatalf("querying api_keys for hashed key: %v", err)
	}
	if stored != 1 {
		t.Errorf("SHA-256 of issued key not stored: got %d rows, want 1", stored)
	}
}

// requireError401 asserts an unauthorized response with the standard
// {"error": "<message>"} shape and no api_key.
func requireError401(t *testing.T, rr *httptest.ResponseRecorder, label string) {
	t.Helper()
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("%s: status = %d, want %d; body: %s", label, rr.Code, http.StatusUnauthorized, rr.Body.String())
	}
	var got map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("%s: error response is not valid JSON: %v; body: %s", label, err, rr.Body.String())
	}
	if got["error"] == "" {
		t.Errorf(`%s: error response missing "error" key: %s`, label, rr.Body.String())
	}
	if got["api_key"] != "" {
		t.Errorf("%s: rejected login must not include an api_key: %s", label, rr.Body.String())
	}
}

func TestLoginWithWrongPasswordIs401(t *testing.T) {
	h, _ := testHandler(t)

	reg := doJSON(t, h, "POST", "/v1/register", `{"name": "alice", "password": "s3cret-pw"}`)
	if reg.Code != http.StatusCreated {
		t.Fatalf("register status = %d, want %d", reg.Code, http.StatusCreated)
	}

	rr := doJSON(t, h, "POST", "/v1/login", `{"name": "alice", "password": "wrong-pw"}`)
	requireError401(t, rr, "wrong password")
}

func TestLoginCLIOnlyUserIsRefused(t *testing.T) {
	h, db := testHandler(t)

	// A CLI-registered user has no password hash.
	if _, err := db.ExecContext(context.Background(),
		"INSERT INTO users (id, created_at, updated_at, name) VALUES (gen_random_uuid(), now(), now(), 'cliuser')",
	); err != nil {
		t.Fatalf("seeding CLI-only user: %v", err)
	}

	rr := doJSON(t, h, "POST", "/v1/login", `{"name": "cliuser", "password": "guessed-pw"}`)
	requireError401(t, rr, "CLI-only user with guessed password")

	// A null hash never matches any password, including empty.
	empty := doJSON(t, h, "POST", "/v1/login", `{"name": "cliuser", "password": ""}`)
	requireError401(t, empty, "CLI-only user with empty password")
}

func TestLoginUnknownUserIs401(t *testing.T) {
	h, _ := testHandler(t)

	rr := doJSON(t, h, "POST", "/v1/login", `{"name": "nobody", "password": "whatever"}`)
	requireError401(t, rr, "unknown user")
}

func TestLoginMalformedRequests(t *testing.T) {
	h, _ := testHandler(t)

	rr := doJSON(t, h, "POST", "/v1/login", `not json`)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("login with non-JSON body: status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
	var got map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil || got["error"] == "" {
		t.Errorf("login with non-JSON body: want error-shape JSON, got: %s", rr.Body.String())
	}

	// Missing fields are just credentials that never match — 401, so the
	// authenticated surface fails closed without leaking which part was bad.
	for _, body := range []string{`{"name": "bob"}`, `{"password": "pw"}`, `{}`} {
		requireError401(t, doJSON(t, h, "POST", "/v1/login", body), "login with body "+body)
	}
}
