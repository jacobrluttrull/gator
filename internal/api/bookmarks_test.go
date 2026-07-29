package api_test

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"testing"
	"time"
)

// seedBookmarkablePost seeds a feed the named user follows plus one post
// in it, and returns that post's url.
func seedBookmarkablePost(t *testing.T, db *sql.DB, userName, title, url string) string {
	t.Helper()
	feedName := userName + "'s Feed"
	seedFeed(t, db, userName, feedName, "https://"+userName+".example/rss")
	seedFollow(t, db, userName, feedName)
	seedPost(t, db, feedName, title, url, time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC))
	return url
}

// getBookmarks fetches /v1/bookmarks and decodes the response body into a
// JSON array.
func getBookmarks(t *testing.T, h http.Handler, key string) []map[string]any {
	t.Helper()
	rr := doAuthed(t, h, "GET", "/v1/bookmarks", "ApiKey "+key)
	if rr.Code != http.StatusOK {
		t.Fatalf("bookmarks status = %d, want %d; body: %s", rr.Code, http.StatusOK, rr.Body.String())
	}
	var got []map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("response is not a JSON array: %v; body: %s", err, rr.Body.String())
	}
	if got == nil {
		t.Fatalf("bookmarks = JSON null, want empty list; body: %s", rr.Body.String())
	}
	return got
}

func TestBookmarkRoundTrip(t *testing.T) {
	h, db := testHandler(t)

	key := registerAndLogin(t, h, "alice", "alice-pw")
	url := seedBookmarkablePost(t, db, "alice", "a post", "https://alice.example/1")

	if got := getBookmarks(t, h, key); len(got) != 0 {
		t.Fatalf("new user's bookmarks = %d entries, want 0; entries: %v", len(got), got)
	}

	rr := doAuthedJSON(t, h, "POST", "/v1/bookmarks", key, `{"url": "`+url+`"}`)
	if rr.Code != http.StatusCreated {
		t.Fatalf("bookmark status = %d, want %d; body: %s", rr.Code, http.StatusCreated, rr.Body.String())
	}
	var created map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &created); err != nil {
		t.Fatalf("bookmark response is not valid JSON: %v; body: %s", err, rr.Body.String())
	}
	if created["post_title"] != "a post" || created["post_url"] != url {
		t.Errorf("bookmark = {post_title: %v, post_url: %v}, want {%q, %q}",
			created["post_title"], created["post_url"], "a post", url)
	}
	if id, _ := created["id"].(string); id == "" {
		t.Errorf("bookmark id missing or empty in response: %s", rr.Body.String())
	}

	got := getBookmarks(t, h, key)
	if len(got) != 1 {
		t.Fatalf("bookmarks after bookmarking = %d entries, want 1; entries: %v", len(got), got)
	}
	if got[0]["title"] != "a post" || got[0]["url"] != url {
		t.Errorf("bookmarks[0] = {title: %v, url: %v}, want the bookmarked post", got[0]["title"], got[0]["url"])
	}

	rr = doAuthedJSON(t, h, "DELETE", "/v1/bookmarks", key, `{"url": "`+url+`"}`)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("unbookmark status = %d, want %d; body: %s", rr.Code, http.StatusNoContent, rr.Body.String())
	}
	if got := getBookmarks(t, h, key); len(got) != 0 {
		t.Errorf("bookmarks after unbookmarking = %d entries, want 0; entries: %v", len(got), got)
	}
}

func TestBookmarkUnknownPostIs404(t *testing.T) {
	h, _ := testHandler(t)

	key := registerAndLogin(t, h, "alice", "alice-pw")

	for _, method := range []string{"POST", "DELETE"} {
		rr := doAuthedJSON(t, h, method, "/v1/bookmarks", key, `{"url": "https://nowhere.example/1"}`)
		if rr.Code != http.StatusNotFound {
			t.Errorf("%s unknown post status = %d, want %d; body: %s", method, rr.Code, http.StatusNotFound, rr.Body.String())
			continue
		}
		var got map[string]string
		if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil || got["error"] == "" {
			t.Errorf("%s unknown post: want error-shape JSON, got: %s", method, rr.Body.String())
		}
	}
}

func TestBookmarkBadBodyIs400(t *testing.T) {
	h, _ := testHandler(t)

	key := registerAndLogin(t, h, "alice", "alice-pw")

	for _, body := range []string{`not json`, `{}`, `{"url": ""}`} {
		rr := doAuthedJSON(t, h, "POST", "/v1/bookmarks", key, body)
		if rr.Code != http.StatusBadRequest {
			t.Errorf("bookmark with body %q: status = %d, want %d; body: %s", body, rr.Code, http.StatusBadRequest, rr.Body.String())
			continue
		}
		var got map[string]string
		if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil || got["error"] == "" {
			t.Errorf("bookmark with body %q: want error-shape JSON, got: %s", body, rr.Body.String())
		}
	}
}

func TestBookmarkTwiceIsConflict(t *testing.T) {
	h, db := testHandler(t)

	key := registerAndLogin(t, h, "alice", "alice-pw")
	url := seedBookmarkablePost(t, db, "alice", "a post", "https://alice.example/1")

	body := `{"url": "` + url + `"}`
	if rr := doAuthedJSON(t, h, "POST", "/v1/bookmarks", key, body); rr.Code != http.StatusCreated {
		t.Fatalf("first bookmark status = %d, want %d; body: %s", rr.Code, http.StatusCreated, rr.Body.String())
	}

	rr := doAuthedJSON(t, h, "POST", "/v1/bookmarks", key, body)
	if rr.Code != http.StatusConflict {
		t.Fatalf("double bookmark status = %d, want %d; body: %s", rr.Code, http.StatusConflict, rr.Body.String())
	}
	var got map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil || got["error"] == "" {
		t.Errorf("double bookmark: want error-shape JSON, got: %s", rr.Body.String())
	}

	if bookmarks := getBookmarks(t, h, key); len(bookmarks) != 1 {
		t.Errorf("bookmarks after double bookmark = %d entries, want 1; entries: %v", len(bookmarks), bookmarks)
	}
}

func TestBookmarksArePerUser(t *testing.T) {
	h, db := testHandler(t)

	aliceKey := registerAndLogin(t, h, "alice", "alice-pw")
	bobKey := registerAndLogin(t, h, "bob", "bob-pw")
	aliceURL := seedBookmarkablePost(t, db, "alice", "alice post", "https://alice.example/1")
	bobURL := seedBookmarkablePost(t, db, "bob", "bob post", "https://bob.example/1")

	if rr := doAuthedJSON(t, h, "POST", "/v1/bookmarks", aliceKey, `{"url": "`+aliceURL+`"}`); rr.Code != http.StatusCreated {
		t.Fatalf("alice bookmark status = %d, want %d; body: %s", rr.Code, http.StatusCreated, rr.Body.String())
	}
	if rr := doAuthedJSON(t, h, "POST", "/v1/bookmarks", bobKey, `{"url": "`+bobURL+`"}`); rr.Code != http.StatusCreated {
		t.Fatalf("bob bookmark status = %d, want %d; body: %s", rr.Code, http.StatusCreated, rr.Body.String())
	}

	for _, tc := range []struct {
		user, key, wantTitle string
	}{
		{"alice", aliceKey, "alice post"},
		{"bob", bobKey, "bob post"},
	} {
		got := getBookmarks(t, h, tc.key)
		if len(got) != 1 {
			t.Fatalf("%s bookmarks = %d entries, want exactly their own 1; entries: %v", tc.user, len(got), got)
		}
		if got[0]["title"] != tc.wantTitle {
			t.Errorf("%s bookmarks[0].title = %v, want %q", tc.user, got[0]["title"], tc.wantTitle)
		}
	}

	// One user's unbookmark leaves the other's bookmark of the same post alone.
	if rr := doAuthedJSON(t, h, "POST", "/v1/bookmarks", bobKey, `{"url": "`+aliceURL+`"}`); rr.Code != http.StatusCreated {
		t.Fatalf("bob bookmarking alice's post status = %d, want %d; body: %s", rr.Code, http.StatusCreated, rr.Body.String())
	}
	if rr := doAuthedJSON(t, h, "DELETE", "/v1/bookmarks", bobKey, `{"url": "`+aliceURL+`"}`); rr.Code != http.StatusNoContent {
		t.Fatalf("bob unbookmark status = %d, want %d; body: %s", rr.Code, http.StatusNoContent, rr.Body.String())
	}
	if got := getBookmarks(t, h, aliceKey); len(got) != 1 || got[0]["title"] != "alice post" {
		t.Errorf("alice's bookmarks after bob unbookmarked the same post = %v, want her 1 bookmark intact", got)
	}
}

func TestBookmarksFailClosed(t *testing.T) {
	h, _ := testHandler(t)

	key := registerAndLogin(t, h, "alice", "alice-pw")
	unknownKey := "deadbeef" + key[8:]

	for _, tt := range []struct {
		name          string
		method        string
		authorization string
	}{
		{"list, no Authorization header", "GET", ""},
		{"list, unknown well-formed key", "GET", "ApiKey " + unknownKey},
		{"create, no Authorization header", "POST", ""},
		{"create, unknown well-formed key", "POST", "ApiKey " + unknownKey},
		{"delete, no Authorization header", "DELETE", ""},
		{"delete, unknown well-formed key", "DELETE", "ApiKey " + unknownKey},
	} {
		t.Run(tt.name, func(t *testing.T) {
			rr := doAuthed(t, h, tt.method, "/v1/bookmarks", tt.authorization)
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
