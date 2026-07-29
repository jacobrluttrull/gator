package api_test

import (
	"encoding/json"
	"net/http"
	"testing"
)

// authWorks reports whether a key still authenticates, failing the test on
// any status other than 200 or 401 (which would mean something other than
// revocation is going on).
func authWorks(t *testing.T, h http.Handler, key string) bool {
	t.Helper()
	rr := doAuthed(t, h, "GET", "/v1/follows", "ApiKey "+key)
	switch rr.Code {
	case http.StatusOK:
		return true
	case http.StatusUnauthorized:
		return false
	default:
		t.Fatalf("GET /v1/follows = %d, want 200 or 401; body: %s", rr.Code, rr.Body.String())
		return false
	}
}

func TestRevokeKeysInvalidatesTheCallersKeys(t *testing.T) {
	h, _ := testHandler(t)

	// Two keys for one user: revocation must take both, not just the one
	// that made the request.
	first := registerAndLogin(t, h, "alice", "alice-pw")
	second := loginForKey(t, h, "alice", "alice-pw")
	if first == second {
		t.Fatal("two logins returned the same key; this test can't tell them apart")
	}
	if !authWorks(t, h, first) || !authWorks(t, h, second) {
		t.Fatal("freshly issued keys don't authenticate; nothing to revoke")
	}

	rr := doAuthed(t, h, "DELETE", "/v1/keys", "ApiKey "+first)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("revoke status = %d, want %d; body: %s", rr.Code, http.StatusNoContent, rr.Body.String())
	}

	if authWorks(t, h, first) {
		t.Error("the key that requested revocation still authenticates")
	}
	if authWorks(t, h, second) {
		t.Error("a second key survived revocation; revoke must take every key the user holds")
	}

	// Revocation is not a lockout: logging in again issues a working key.
	third := loginForKey(t, h, "alice", "alice-pw")
	if !authWorks(t, h, third) {
		t.Error("a key issued after revocation doesn't authenticate")
	}
}

func TestRevokeKeysIsPerUser(t *testing.T) {
	h, _ := testHandler(t)

	aliceKey := registerAndLogin(t, h, "alice", "alice-pw")
	bobKey := registerAndLogin(t, h, "bob", "bob-pw")

	if rr := doAuthed(t, h, "DELETE", "/v1/keys", "ApiKey "+aliceKey); rr.Code != http.StatusNoContent {
		t.Fatalf("alice revoke status = %d, want %d; body: %s", rr.Code, http.StatusNoContent, rr.Body.String())
	}

	if authWorks(t, h, aliceKey) {
		t.Error("alice's key survived her own revocation")
	}
	if !authWorks(t, h, bobKey) {
		t.Error("bob's key was revoked by alice's request")
	}
}

// A revoked key must not be able to revoke again — the second call is an
// ordinary unauthenticated request.
func TestRevokeTwiceIsUnauthorized(t *testing.T) {
	h, _ := testHandler(t)

	key := registerAndLogin(t, h, "alice", "alice-pw")
	if rr := doAuthed(t, h, "DELETE", "/v1/keys", "ApiKey "+key); rr.Code != http.StatusNoContent {
		t.Fatalf("first revoke status = %d, want %d", rr.Code, http.StatusNoContent)
	}

	rr := doAuthed(t, h, "DELETE", "/v1/keys", "ApiKey "+key)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("second revoke status = %d, want %d; body: %s", rr.Code, http.StatusUnauthorized, rr.Body.String())
	}
	var got map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil || got["error"] == "" {
		t.Errorf("want error-shape JSON, got: %s", rr.Body.String())
	}
}

func TestRevokeKeysFailsClosed(t *testing.T) {
	h, _ := testHandler(t)

	key := registerAndLogin(t, h, "alice", "alice-pw")

	for _, tt := range []struct {
		name          string
		authorization string
	}{
		{"no Authorization header", ""},
		{"unknown well-formed key", "ApiKey " + "deadbeef" + key[8:]},
	} {
		t.Run(tt.name, func(t *testing.T) {
			rr := doAuthed(t, h, "DELETE", "/v1/keys", tt.authorization)
			if rr.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want %d; body: %s", rr.Code, http.StatusUnauthorized, rr.Body.String())
			}
			var got map[string]string
			if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil || got["error"] == "" {
				t.Errorf("want error-shape JSON, got: %s", rr.Body.String())
			}
		})
	}

	// The rejected attempts left the real key alone.
	if !authWorks(t, h, key) {
		t.Error("an unauthenticated revoke attempt revoked the caller's key anyway")
	}
}
