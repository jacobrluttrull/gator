package api_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	_ "github.com/lib/pq"

	"github.com/jacobrluttrull/gator/internal/api"
	"github.com/jacobrluttrull/gator/internal/database"
	"github.com/jacobrluttrull/gator/internal/testsupport"
)

// testHandler opens the shared test database and returns the /v1 API
// handler plus the underlying DB for test seeding.
func testHandler(t *testing.T) (http.Handler, *sql.DB) {
	t.Helper()
	db := testsupport.OpenTestDB(t)
	return api.New(database.New(db)), db
}

func doJSON(t *testing.T, h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequestWithContext(context.Background(), method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr
}

// getJSONArray issues an authenticated GET and decodes a list response
// body, failing the test on a non-200 or on JSON null (an empty list must
// encode as []).
func getJSONArray(t *testing.T, h http.Handler, key, path string) []map[string]any {
	t.Helper()
	rr := doAuthed(t, h, "GET", path, "ApiKey "+key)
	if rr.Code != http.StatusOK {
		t.Fatalf("GET %s status = %d, want %d; body: %s", path, rr.Code, http.StatusOK, rr.Body.String())
	}
	var got []map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("GET %s response is not a JSON array: %v; body: %s", path, err, rr.Body.String())
	}
	if got == nil {
		t.Fatalf("GET %s = JSON null, want empty list; body: %s", path, rr.Body.String())
	}
	return got
}

// TestUnmatchedRoutesUseTheErrorShape pins the JSON error contract for
// requests no route claims: before this, a typo'd path or the wrong verb
// fell through to the stdlib's plain-text "404 page not found" /
// "Method Not Allowed", which a client decoding {"error": ...} can't read.
func TestUnmatchedRoutesUseTheErrorShape(t *testing.T) {
	h, _ := testHandler(t)

	for _, tt := range []struct {
		name, method, path string
		wantStatus         int
		wantAllow          string
	}{
		{"unknown path", "GET", "/v1/nonsense", http.StatusNotFound, ""},
		{"unversioned path", "GET", "/", http.StatusNotFound, ""},
		{"wrong verb on a public route", "GET", "/v1/register", http.StatusMethodNotAllowed, "POST"},
		{"wrong verb on an authed route", "PATCH", "/v1/posts", http.StatusMethodNotAllowed, "GET"},
		{"wrong verb, multi-method route", "PUT", "/v1/bookmarks", http.StatusMethodNotAllowed, "DELETE, GET, POST"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			rr := doAuthed(t, h, tt.method, tt.path, "")
			if rr.Code != tt.wantStatus {
				t.Fatalf("%s %s status = %d, want %d; body: %s", tt.method, tt.path, rr.Code, tt.wantStatus, rr.Body.String())
			}
			if ct := rr.Header().Get("Content-Type"); ct != "application/json" {
				t.Errorf("Content-Type = %q, want application/json", ct)
			}
			var got map[string]string
			if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil || got["error"] == "" {
				t.Errorf("want error-shape JSON, got: %s", rr.Body.String())
			}
			if allow := rr.Header().Get("Allow"); allow != tt.wantAllow {
				t.Errorf("Allow = %q, want %q", allow, tt.wantAllow)
			}
		})
	}
}

// A 405 must not double as an auth oracle: an unauthenticated caller
// using the wrong verb learns the route exists either way, but the
// authenticated routes must still never run without a key.
func TestUnmatchedMethodDoesNotBypassAuth(t *testing.T) {
	h, _ := testHandler(t)

	rr := doAuthed(t, h, "GET", "/v1/bookmarks", "")
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("GET /v1/bookmarks without a key = %d, want %d; body: %s", rr.Code, http.StatusUnauthorized, rr.Body.String())
	}
}

func TestRegisterCreatesUser(t *testing.T) {
	h, _ := testHandler(t)

	rr := doJSON(t, h, "POST", "/v1/register", `{"name": "alice", "password": "s3cret-pw"}`)

	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body: %s", rr.Code, http.StatusCreated, rr.Body.String())
	}
	var got map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("response is not valid JSON: %v; body: %s", err, rr.Body.String())
	}
	if got["name"] != "alice" {
		t.Errorf("name = %v, want %q", got["name"], "alice")
	}
	if id, _ := got["id"].(string); id == "" {
		t.Errorf("id missing or empty in response: %s", rr.Body.String())
	}
	lower := strings.ToLower(rr.Body.String())
	if strings.Contains(lower, "password") || strings.Contains(lower, "s3cret-pw") {
		t.Errorf("response leaks password material: %s", rr.Body.String())
	}
}

// TestRegisterOverlongPasswordIs400 covers bcrypt's 72-byte input limit,
// which password-manager output routinely exceeds: it's bad input, so it
// must not surface as "couldn't hash password" with a 500.
func TestRegisterOverlongPasswordIs400(t *testing.T) {
	h, _ := testHandler(t)

	rr := doJSON(t, h, "POST", "/v1/register",
		`{"name": "alice", "password": "`+strings.Repeat("a", 73)+`"}`)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("register with a 73-byte password: status = %d, want %d; body: %s",
			rr.Code, http.StatusBadRequest, rr.Body.String())
	}
	var got map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil || got["error"] == "" {
		t.Errorf("want error-shape JSON, got: %s", rr.Body.String())
	}

	// The boundary itself still registers, and the rejected attempt left
	// no half-created user behind to collide with it.
	rr = doJSON(t, h, "POST", "/v1/register",
		`{"name": "alice", "password": "`+strings.Repeat("a", 72)+`"}`)
	if rr.Code != http.StatusCreated {
		t.Errorf("register with a 72-byte password: status = %d, want %d; body: %s",
			rr.Code, http.StatusCreated, rr.Body.String())
	}
}

func TestRegisterDuplicateNameIsClean4xx(t *testing.T) {
	h, _ := testHandler(t)

	first := doJSON(t, h, "POST", "/v1/register", `{"name": "alice", "password": "s3cret-pw"}`)
	if first.Code != http.StatusCreated {
		t.Fatalf("first register status = %d, want %d", first.Code, http.StatusCreated)
	}

	second := doJSON(t, h, "POST", "/v1/register", `{"name": "alice", "password": "other-pw"}`)
	if second.Code != http.StatusConflict {
		t.Fatalf("duplicate register status = %d, want %d; body: %s", second.Code, http.StatusConflict, second.Body.String())
	}
	var got map[string]string
	if err := json.Unmarshal(second.Body.Bytes(), &got); err != nil {
		t.Fatalf("error response is not valid JSON: %v; body: %s", err, second.Body.String())
	}
	if got["error"] == "" {
		t.Errorf(`error response missing "error" key: %s`, second.Body.String())
	}
}

func TestRegisterMissingFieldsIs400(t *testing.T) {
	h, _ := testHandler(t)

	for _, body := range []string{`{"name": "bob"}`, `{"password": "pw"}`, `not json`} {
		rr := doJSON(t, h, "POST", "/v1/register", body)
		if rr.Code != http.StatusBadRequest {
			t.Errorf("register with body %q: status = %d, want %d", body, rr.Code, http.StatusBadRequest)
		}
		var got map[string]string
		if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil || got["error"] == "" {
			t.Errorf("register with body %q: want error-shape JSON, got: %s", body, rr.Body.String())
		}
	}
}
