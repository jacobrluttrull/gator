package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// doAuthedJSON issues a request with a JSON body and an ApiKey
// Authorization header.
func doAuthedJSON(t *testing.T, h http.Handler, method, path, key, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequestWithContext(context.Background(), method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "ApiKey "+key)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr
}

func TestAddFeedReturnsItAndFollowsIt(t *testing.T) {
	h, _ := testHandler(t)

	key := registerAndLogin(t, h, "alice", "s3cret-pw")

	rr := doAuthedJSON(t, h, "POST", "/v1/feeds", key,
		`{"name": "Alice's Feed", "url": "https://alice.example/rss"}`)
	if rr.Code != http.StatusCreated {
		t.Fatalf("add feed status = %d, want %d; body: %s", rr.Code, http.StatusCreated, rr.Body.String())
	}
	var got struct {
		Feed struct {
			ID   string `json:"id"`
			Name string `json:"name"`
			URL  string `json:"url"`
		} `json:"feed"`
		FeedFollow struct {
			UserName string `json:"user_name"`
			FeedName string `json:"feed_name"`
		} `json:"feed_follow"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("response is not valid JSON: %v; body: %s", err, rr.Body.String())
	}
	if got.Feed.Name != "Alice's Feed" || got.Feed.URL != "https://alice.example/rss" {
		t.Errorf("feed = {name: %q, url: %q}, want the added feed; body: %s",
			got.Feed.Name, got.Feed.URL, rr.Body.String())
	}
	if got.Feed.ID == "" {
		t.Errorf("feed id missing or empty in response: %s", rr.Body.String())
	}
	if got.FeedFollow.UserName != "alice" || got.FeedFollow.FeedName != "Alice's Feed" {
		t.Errorf("feed_follow = {user_name: %q, feed_name: %q}, want {%q, %q}",
			got.FeedFollow.UserName, got.FeedFollow.FeedName, "alice", "Alice's Feed")
	}

	// The adder is now following the feed: it shows up in GET /v1/follows.
	follows := doAuthed(t, h, "GET", "/v1/follows", "ApiKey "+key)
	if follows.Code != http.StatusOK {
		t.Fatalf("follows status = %d, want %d; body: %s", follows.Code, http.StatusOK, follows.Body.String())
	}
	var followed []struct {
		FeedName string `json:"feed_name"`
	}
	if err := json.Unmarshal(follows.Body.Bytes(), &followed); err != nil {
		t.Fatalf("follows is not a JSON array: %v; body: %s", err, follows.Body.String())
	}
	if len(followed) != 1 || followed[0].FeedName != "Alice's Feed" {
		t.Errorf("follows after add = %s, want exactly the added feed", follows.Body.String())
	}
}

func TestAddFeedDuplicateURLIsClean409(t *testing.T) {
	h, _ := testHandler(t)

	aliceKey := registerAndLogin(t, h, "alice", "alice-pw")
	bobKey := registerAndLogin(t, h, "bob", "bob-pw")

	first := doAuthedJSON(t, h, "POST", "/v1/feeds", aliceKey,
		`{"name": "Alice's Feed", "url": "https://alice.example/rss"}`)
	if first.Code != http.StatusCreated {
		t.Fatalf("first add status = %d, want %d; body: %s", first.Code, http.StatusCreated, first.Body.String())
	}

	// Re-adding the same URL fails cleanly — whoever tries it.
	for _, tc := range []struct{ user, key string }{{"alice", aliceKey}, {"bob", bobKey}} {
		rr := doAuthedJSON(t, h, "POST", "/v1/feeds", tc.key,
			`{"name": "Same URL Again", "url": "https://alice.example/rss"}`)
		if rr.Code != http.StatusConflict {
			t.Fatalf("%s duplicate add status = %d, want %d; body: %s", tc.user, rr.Code, http.StatusConflict, rr.Body.String())
		}
		var got map[string]string
		if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil || got["error"] == "" {
			t.Errorf("%s duplicate add: want error-shape JSON, got: %s", tc.user, rr.Body.String())
		}
	}
}

func TestAddFeedMissingFieldsIs400(t *testing.T) {
	h, _ := testHandler(t)

	key := registerAndLogin(t, h, "alice", "s3cret-pw")

	for _, body := range []string{`{"name": "No URL"}`, `{"url": "https://a.example/rss"}`, `not json`} {
		rr := doAuthedJSON(t, h, "POST", "/v1/feeds", key, body)
		if rr.Code != http.StatusBadRequest {
			t.Errorf("add feed with body %q: status = %d, want %d", body, rr.Code, http.StatusBadRequest)
		}
		var got map[string]string
		if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil || got["error"] == "" {
			t.Errorf("add feed with body %q: want error-shape JSON, got: %s", body, rr.Body.String())
		}
	}
}

func TestFeedAndFollowRoutesFailClosed(t *testing.T) {
	h, _ := testHandler(t)

	routes := []struct{ method, path string }{
		{"POST", "/v1/feeds"},
		{"GET", "/v1/feeds"},
		{"POST", "/v1/follows"},
		{"DELETE", "/v1/follows"},
	}
	for _, route := range routes {
		t.Run(route.method+" "+route.path, func(t *testing.T) {
			for _, authorization := range []string{"", "ApiKey not-a-real-key"} {
				req := httptest.NewRequestWithContext(context.Background(), route.method, route.path,
					strings.NewReader(`{"name": "n", "url": "https://a.example/rss"}`))
				req.Header.Set("Content-Type", "application/json")
				if authorization != "" {
					req.Header.Set("Authorization", authorization)
				}
				rr := httptest.NewRecorder()
				h.ServeHTTP(rr, req)
				if rr.Code != http.StatusUnauthorized {
					t.Fatalf("auth %q: status = %d, want %d; body: %s", authorization, rr.Code, http.StatusUnauthorized, rr.Body.String())
				}
				var got map[string]string
				if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil || got["error"] == "" {
					t.Errorf("auth %q: want error-shape JSON, got: %s", authorization, rr.Body.String())
				}
			}
		})
	}
}

func TestListFeedsShowsSharedPool(t *testing.T) {
	h, _ := testHandler(t)

	aliceKey := registerAndLogin(t, h, "alice", "alice-pw")
	bobKey := registerAndLogin(t, h, "bob", "bob-pw")

	if rr := doAuthedJSON(t, h, "POST", "/v1/feeds", aliceKey,
		`{"name": "Alice's Feed", "url": "https://alice.example/rss"}`); rr.Code != http.StatusCreated {
		t.Fatalf("alice add feed status = %d, want %d; body: %s", rr.Code, http.StatusCreated, rr.Body.String())
	}
	if rr := doAuthedJSON(t, h, "POST", "/v1/feeds", bobKey,
		`{"name": "Bob's Feed", "url": "https://bob.example/rss"}`); rr.Code != http.StatusCreated {
		t.Fatalf("bob add feed status = %d, want %d; body: %s", rr.Code, http.StatusCreated, rr.Body.String())
	}

	// The pool is shared: alice sees bob's feed too.
	rr := doAuthed(t, h, "GET", "/v1/feeds", "ApiKey "+aliceKey)
	if rr.Code != http.StatusOK {
		t.Fatalf("list feeds status = %d, want %d; body: %s", rr.Code, http.StatusOK, rr.Body.String())
	}
	var got []struct {
		FeedName string `json:"feed_name"`
		FeedUrl  string `json:"feed_url"`
		Username string `json:"username"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("feeds is not a JSON array: %v; body: %s", err, rr.Body.String())
	}
	if len(got) != 2 {
		t.Fatalf("feeds = %d entries, want the whole pool of 2; body: %s", len(got), rr.Body.String())
	}
	byName := map[string]struct{ url, adder string }{}
	for _, f := range got {
		byName[f.FeedName] = struct{ url, adder string }{f.FeedUrl, f.Username}
	}
	if f, ok := byName["Alice's Feed"]; !ok || f.url != "https://alice.example/rss" || f.adder != "alice" {
		t.Errorf("pool missing alice's feed with url and adder; body: %s", rr.Body.String())
	}
	if f, ok := byName["Bob's Feed"]; !ok || f.url != "https://bob.example/rss" || f.adder != "bob" {
		t.Errorf("pool missing bob's feed with url and adder; body: %s", rr.Body.String())
	}
}
