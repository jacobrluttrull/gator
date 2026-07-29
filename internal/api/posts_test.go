package api_test

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"testing"
	"time"
)

// seedPost inserts a post into the named feed, bypassing the API.
func seedPost(t *testing.T, db *sql.DB, feedName, title, url string, publishedAt time.Time) {
	t.Helper()
	seedExec(t, db, "seeding post "+title, `
		INSERT INTO posts (id, created_at, updated_at, title, url, description, published_at, feed_id)
		SELECT gen_random_uuid(), now(), now(), $2, $3, '', $4, feeds.id
		FROM feeds WHERE feeds.name = $1`,
		feedName, title, url, publishedAt,
	)
}

// getPosts fetches /v1/posts (plus optional query string) and decodes the
// response body into a JSON array.
func getPosts(t *testing.T, h http.Handler, key, query string) []map[string]any {
	t.Helper()
	return getJSONArray(t, h, key, "/v1/posts"+query)
}

// seedThreePosts seeds one followed feed for the named user with three
// posts whose published_at order is the reverse of their insertion order,
// so newest-first output can't be mistaken for insertion order.
func seedThreePosts(t *testing.T, db *sql.DB, userName string) {
	t.Helper()
	seedFeed(t, db, userName, "The Feed", "https://feed.example/rss")
	seedFollow(t, db, userName, "The Feed")
	seedPost(t, db, "The Feed", "middle", "https://feed.example/2", time.Date(2026, 7, 2, 0, 0, 0, 0, time.UTC))
	seedPost(t, db, "The Feed", "oldest", "https://feed.example/1", time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC))
	seedPost(t, db, "The Feed", "newest", "https://feed.example/3", time.Date(2026, 7, 3, 0, 0, 0, 0, time.UTC))
}

func titles(posts []map[string]any) []string {
	out := make([]string, 0, len(posts))
	for _, p := range posts {
		title, _ := p["title"].(string)
		out = append(out, title)
	}
	return out
}

func TestPostsDefaultLimitNewestFirst(t *testing.T) {
	h, db := testHandler(t)

	key := registerAndLogin(t, h, "alice", "alice-pw")
	seedThreePosts(t, db, "alice")

	got := getPosts(t, h, key, "")
	want := []string{"newest", "middle"}
	if gotTitles := titles(got); len(gotTitles) != len(want) || gotTitles[0] != want[0] || gotTitles[1] != want[1] {
		t.Errorf("posts without limit param = %v, want newest 2 in order %v", gotTitles, want)
	}
}

func TestPostsLimitParamOverridesDefault(t *testing.T) {
	h, db := testHandler(t)

	key := registerAndLogin(t, h, "alice", "alice-pw")
	seedThreePosts(t, db, "alice")

	for _, tc := range []struct {
		query string
		want  []string
	}{
		{"?limit=1", []string{"newest"}},
		{"?limit=3", []string{"newest", "middle", "oldest"}},
	} {
		got := titles(getPosts(t, h, key, tc.query))
		if len(got) != len(tc.want) {
			t.Fatalf("posts%s = %v, want %v", tc.query, got, tc.want)
		}
		for i := range tc.want {
			if got[i] != tc.want[i] {
				t.Fatalf("posts%s = %v, want %v", tc.query, got, tc.want)
			}
		}
	}
}

func TestPostsEmptyForUserWithNoFollowings(t *testing.T) {
	h, _ := testHandler(t)

	key := registerAndLogin(t, h, "alice", "alice-pw")

	rr := doAuthed(t, h, "GET", "/v1/posts", "ApiKey "+key)
	if rr.Code != http.StatusOK {
		t.Fatalf("posts status = %d, want %d; body: %s", rr.Code, http.StatusOK, rr.Body.String())
	}
	var got []map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("response is not a JSON array: %v; body: %s", err, rr.Body.String())
	}
	if got == nil {
		t.Fatalf("new user's posts = JSON null, want empty list; body: %s", rr.Body.String())
	}
	if len(got) != 0 {
		t.Errorf("new user's posts = %d entries, want 0; body: %s", len(got), rr.Body.String())
	}
}

func TestPostsFailsClosed(t *testing.T) {
	h, _ := testHandler(t)

	key := registerAndLogin(t, h, "alice", "s3cret-pw")

	for _, tt := range []struct {
		name          string
		authorization string
	}{
		{"no Authorization header", ""},
		{"unknown well-formed key", "ApiKey " + "deadbeef" + key[8:]},
	} {
		t.Run(tt.name, func(t *testing.T) {
			rr := doAuthed(t, h, "GET", "/v1/posts", tt.authorization)
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

func TestPostsInvalidLimitIs400(t *testing.T) {
	h, _ := testHandler(t)

	key := registerAndLogin(t, h, "alice", "alice-pw")

	for _, query := range []string{"?limit=abc", "?limit=1.5", "?limit=0", "?limit=-1"} {
		rr := doAuthed(t, h, "GET", "/v1/posts"+query, "ApiKey "+key)
		if rr.Code != http.StatusBadRequest {
			t.Errorf("posts%s status = %d, want %d; body: %s", query, rr.Code, http.StatusBadRequest, rr.Body.String())
			continue
		}
		var got map[string]string
		if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil || got["error"] == "" {
			t.Errorf("posts%s: want error-shape JSON, got: %s", query, rr.Body.String())
		}
	}
}

func TestPostsListsOnlyFollowedFeeds(t *testing.T) {
	h, db := testHandler(t)

	aliceKey := registerAndLogin(t, h, "alice", "alice-pw")
	bobKey := registerAndLogin(t, h, "bob", "bob-pw")
	// Disjoint Followings: each user follows only their own feed, so any
	// post from the other's feed in the response is a leak.
	seedFeed(t, db, "alice", "Alice's Feed", "https://alice.example/rss")
	seedFeed(t, db, "bob", "Bob's Feed", "https://bob.example/rss")
	seedFollow(t, db, "alice", "Alice's Feed")
	seedFollow(t, db, "bob", "Bob's Feed")
	seedPost(t, db, "Alice's Feed", "alice post", "https://alice.example/1", time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC))
	seedPost(t, db, "Bob's Feed", "bob post", "https://bob.example/1", time.Date(2026, 7, 2, 0, 0, 0, 0, time.UTC))

	for _, tc := range []struct {
		user, key, wantTitle string
	}{
		{"alice", aliceKey, "alice post"},
		{"bob", bobKey, "bob post"},
	} {
		got := getPosts(t, h, tc.key, "")
		if len(got) != 1 {
			t.Fatalf("%s posts = %d entries, want exactly their own 1; entries: %v", tc.user, len(got), got)
		}
		if got[0]["title"] != tc.wantTitle {
			t.Errorf("%s posts[0].title = %v, want %q", tc.user, got[0]["title"], tc.wantTitle)
		}
	}
}
