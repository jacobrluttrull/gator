package api_test

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"
)

// search fetches /v1/search with the given query string and decodes the
// response body into a JSON array.
func search(t *testing.T, h http.Handler, key, query string) []map[string]any {
	t.Helper()
	return getJSONArray(t, h, key, "/v1/search"+query)
}

func TestSearchFindsMisspelledTitle(t *testing.T) {
	h, db := testHandler(t)

	key := registerAndLogin(t, h, "alice", "alice-pw")
	feedName := seedFollowedPost(t, db, "alice", "Introducing Golang Generics", "https://alice.example/1")
	seedPost(t, db, feedName, "Kubernetes Networking", "https://alice.example/2", time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC))

	// Misspelled and differently cased: trigram similarity still matches.
	for _, query := range []string{"?q=goland+generics", "?q=GOLAND+GENERICS"} {
		got := search(t, h, key, query)
		if len(got) != 1 {
			t.Fatalf("search%s = %d entries, want just the Golang post; entries: %v", query, len(got), got)
		}
		if got[0]["title"] != "Introducing Golang Generics" {
			t.Errorf("search%s [0].title = %v, want %q", query, got[0]["title"], "Introducing Golang Generics")
		}
		if _, ok := got[0]["sim"].(float64); !ok {
			t.Errorf("search%s [0] missing numeric sim: %v", query, got[0])
		}
	}
}

func TestSearchOnlyCoversFollowedFeeds(t *testing.T) {
	h, db := testHandler(t)

	aliceKey := registerAndLogin(t, h, "alice", "alice-pw")
	bobKey := registerAndLogin(t, h, "bob", "bob-pw")
	// Disjoint Followings: each user follows only their own feed, so a hit
	// from the other's feed is a leak.
	seedFollowedPost(t, db, "alice", "Rust Ownership Explained", "https://alice.example/1")
	seedFollowedPost(t, db, "bob", "Rust Ownership Revisited", "https://bob.example/1")

	for _, tc := range []struct {
		user, key, wantTitle string
	}{
		{"alice", aliceKey, "Rust Ownership Explained"},
		{"bob", bobKey, "Rust Ownership Revisited"},
	} {
		got := search(t, h, tc.key, "?q=rust+ownershp")
		if len(got) != 1 {
			t.Fatalf("%s search = %d entries, want exactly their own 1; entries: %v", tc.user, len(got), got)
		}
		if got[0]["title"] != tc.wantTitle {
			t.Errorf("%s search [0].title = %v, want %q", tc.user, got[0]["title"], tc.wantTitle)
		}
	}
}

func TestSearchNoMatchesIsEmptyList(t *testing.T) {
	h, db := testHandler(t)

	key := registerAndLogin(t, h, "alice", "alice-pw")
	seedFollowedPost(t, db, "alice", "Introducing Golang Generics", "https://alice.example/1")

	if got := search(t, h, key, "?q=kubernetes+networking"); len(got) != 0 {
		t.Errorf("search for an unrelated term = %d entries, want 0; entries: %v", len(got), got)
	}
}

func TestSearchRespectsLimit(t *testing.T) {
	h, db := testHandler(t)

	key := registerAndLogin(t, h, "alice", "alice-pw")
	feedName := seedFollowedPost(t, db, "alice", "Rust Ownership Explained", "https://alice.example/1")
	seedPost(t, db, feedName, "Rust Ownership Revisited", "https://alice.example/2", time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC))

	if got := search(t, h, key, "?q=rust+ownershp"); len(got) != 2 {
		t.Fatalf("search without limit = %d entries, want both matches; entries: %v", len(got), got)
	}
	if got := search(t, h, key, "?q=rust+ownershp&limit=1"); len(got) != 1 {
		t.Errorf("search with limit=1 = %d entries, want 1; entries: %v", len(got), got)
	}
}

func TestSearchMissingQueryIs400(t *testing.T) {
	h, _ := testHandler(t)

	key := registerAndLogin(t, h, "alice", "alice-pw")

	for _, query := range []string{"", "?q=", "?q=+", "?limit=5"} {
		rr := doAuthed(t, h, "GET", "/v1/search"+query, "ApiKey "+key)
		if rr.Code != http.StatusBadRequest {
			t.Errorf("search%q status = %d, want %d; body: %s", query, rr.Code, http.StatusBadRequest, rr.Body.String())
			continue
		}
		var got map[string]string
		if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil || got["error"] == "" {
			t.Errorf("search%q: want error-shape JSON, got: %s", query, rr.Body.String())
		}
	}
}

func TestSearchInvalidLimitIs400(t *testing.T) {
	h, _ := testHandler(t)

	key := registerAndLogin(t, h, "alice", "alice-pw")

	// 2147483648 is int32 overflow: it must not wrap into a negative LIMIT.
	for _, query := range []string{"?q=rust&limit=abc", "?q=rust&limit=0", "?q=rust&limit=-1", "?q=rust&limit=2147483648"} {
		rr := doAuthed(t, h, "GET", "/v1/search"+query, "ApiKey "+key)
		if rr.Code != http.StatusBadRequest {
			t.Errorf("search%s status = %d, want %d; body: %s", query, rr.Code, http.StatusBadRequest, rr.Body.String())
			continue
		}
		var got map[string]string
		if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil || got["error"] == "" {
			t.Errorf("search%s: want error-shape JSON, got: %s", query, rr.Body.String())
		}
	}
}

func TestSearchFailsClosed(t *testing.T) {
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
			rr := doAuthed(t, h, "GET", "/v1/search?q=rust", tt.authorization)
			if rr.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want %d; body: %s", rr.Code, http.StatusUnauthorized, rr.Body.String())
			}
			var got map[string]string
			if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil || got["error"] == "" {
				t.Errorf("want error-shape JSON, got: %s", rr.Body.String())
			}
		})
	}
}
