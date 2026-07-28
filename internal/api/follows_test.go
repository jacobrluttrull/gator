package api_test

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// registerAndLogin registers a user over the API and logs them in,
// returning their API key.
func registerAndLogin(t *testing.T, h http.Handler, name, password string) string {
	t.Helper()
	body := `{"name": "` + name + `", "password": "` + password + `"}`
	if rr := doJSON(t, h, "POST", "/v1/register", body); rr.Code != http.StatusCreated {
		t.Fatalf("register %s status = %d, want %d; body: %s", name, rr.Code, http.StatusCreated, rr.Body.String())
	}
	rr := doJSON(t, h, "POST", "/v1/login", body)
	if rr.Code != http.StatusOK {
		t.Fatalf("login %s status = %d, want %d; body: %s", name, rr.Code, http.StatusOK, rr.Body.String())
	}
	var got struct {
		APIKey string `json:"api_key"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil || got.APIKey == "" {
		t.Fatalf("login %s returned no api_key: %s", name, rr.Body.String())
	}
	return got.APIKey
}

func doAuthed(t *testing.T, h http.Handler, method, path, authorization string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	if authorization != "" {
		req.Header.Set("Authorization", authorization)
	}
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr
}

// seedExec runs a seed statement and fails the test if it matched no rows
// (e.g. a typo'd user or feed name in an INSERT ... SELECT).
func seedExec(t *testing.T, db *sql.DB, label, query string, args ...any) {
	t.Helper()
	res, err := db.Exec(query, args...)
	if err != nil {
		t.Fatalf("%s: %v", label, err)
	}
	if n, err := res.RowsAffected(); err != nil || n == 0 {
		t.Fatalf("%s: seeded no rows", label)
	}
}

// seedFeed inserts a feed owned by the named user, bypassing the API.
// Creating a feed is not following it — Followings are seeded separately.
func seedFeed(t *testing.T, db *sql.DB, ownerName, feedName, feedURL string) {
	t.Helper()
	seedExec(t, db, "seeding feed "+feedName, `
		INSERT INTO feeds (id, created_at, updated_at, name, url, user_id)
		SELECT gen_random_uuid(), now(), now(), $2, $3, users.id
		FROM users WHERE users.name = $1`,
		ownerName, feedName, feedURL,
	)
}

// seedFollow subscribes the named user to the named feed, bypassing the
// API (there is no follow endpoint yet).
func seedFollow(t *testing.T, db *sql.DB, userName, feedName string) {
	t.Helper()
	seedExec(t, db, "seeding follow of "+feedName+" for "+userName, `
		INSERT INTO feed_follows (id, created_at, updated_at, user_id, feed_id)
		SELECT gen_random_uuid(), now(), now(), users.id, feeds.id
		FROM users, feeds WHERE users.name = $1 AND feeds.name = $2`,
		userName, feedName,
	)
}

func TestFollowsRoundTripEmptyForNewUser(t *testing.T) {
	h, _ := testHandler(t)

	key := registerAndLogin(t, h, "alice", "s3cret-pw")

	rr := doAuthed(t, h, "GET", "/v1/follows", "ApiKey "+key)
	if rr.Code != http.StatusOK {
		t.Fatalf("follows status = %d, want %d; body: %s", rr.Code, http.StatusOK, rr.Body.String())
	}
	var got []map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("response is not a JSON array: %v; body: %s", err, rr.Body.String())
	}
	if got == nil {
		t.Fatalf("new user's follows = JSON null, want empty list; body: %s", rr.Body.String())
	}
	if len(got) != 0 {
		t.Errorf("new user's follows = %d entries, want 0; body: %s", len(got), rr.Body.String())
	}
}

func TestFollowsListsOnlyOwnFollowings(t *testing.T) {
	h, db := testHandler(t)

	aliceKey := registerAndLogin(t, h, "alice", "alice-pw")
	bobKey := registerAndLogin(t, h, "bob", "bob-pw")
	// Followings are subscriptions, not authorship: bob creates a feed he
	// doesn't follow and follows alice's instead, so an implementation
	// that listed created feeds would show bob the wrong feed.
	seedFeed(t, db, "alice", "Alice's Feed", "https://alice.example/rss")
	seedFeed(t, db, "bob", "Bob's Feed", "https://bob.example/rss")
	seedFollow(t, db, "alice", "Alice's Feed")
	seedFollow(t, db, "bob", "Alice's Feed")

	for _, tc := range []struct {
		user, key, wantFeed string
	}{
		{"alice", aliceKey, "Alice's Feed"},
		{"bob", bobKey, "Alice's Feed"},
	} {
		rr := doAuthed(t, h, "GET", "/v1/follows", "ApiKey "+tc.key)
		if rr.Code != http.StatusOK {
			t.Fatalf("%s follows status = %d, want %d; body: %s", tc.user, rr.Code, http.StatusOK, rr.Body.String())
		}
		var got []struct {
			UserName string `json:"user_name"`
			FeedName string `json:"feed_name"`
		}
		if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
			t.Fatalf("%s follows is not a JSON array: %v; body: %s", tc.user, err, rr.Body.String())
		}
		if len(got) != 1 {
			t.Fatalf("%s follows = %d entries, want exactly their own 1; body: %s", tc.user, len(got), rr.Body.String())
		}
		if got[0].UserName != tc.user || got[0].FeedName != tc.wantFeed {
			t.Errorf("%s follows = {user_name: %q, feed_name: %q}, want {%q, %q}",
				tc.user, got[0].UserName, got[0].FeedName, tc.user, tc.wantFeed)
		}
	}
}

func TestFollowsFailsClosed(t *testing.T) {
	h, _ := testHandler(t)

	key := registerAndLogin(t, h, "alice", "s3cret-pw")

	tests := []struct {
		name          string
		authorization string
	}{
		{"no Authorization header", ""},
		{"wrong scheme", "Bearer " + key},
		{"scheme without key", "ApiKey"},
		{"scheme with empty key", "ApiKey "},
		{"trailing garbage", "ApiKey " + key + " extra"},
		{"unknown well-formed key", "ApiKey " + "deadbeef" + key[8:]},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rr := doAuthed(t, h, "GET", "/v1/follows", tt.authorization)
			if rr.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want %d; body: %s", rr.Code, http.StatusUnauthorized, rr.Body.String())
			}
			var got map[string]string
			if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
				t.Fatalf("error response is not valid JSON: %v; body: %s", err, rr.Body.String())
			}
			if got["error"] == "" {
				t.Errorf(`error response missing "error" key: %s`, rr.Body.String())
			}
		})
	}
}
