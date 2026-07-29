package api

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/lib/pq"

	"github.com/jacobrluttrull/gator/internal/database"
)

func isUniqueViolation(err error) bool {
	var pqErr *pq.Error
	return errors.As(err, &pqErr) && pqErr.Code.Name() == "unique_violation"
}

// New returns the /v1 API handler backed by the given DB layer.
//
// The routes are a table rather than a run of mux.HandleFunc calls
// because handleUnmatched needs the same method/path pairs to answer 405
// — one list keeps the Allow header from drifting from what's mounted.
func New(db *database.Queries) http.Handler {
	routes := []struct {
		method, path string
		handler      http.HandlerFunc
	}{
		{"POST", "/v1/register", handleRegister(db)},
		{"POST", "/v1/login", handleLogin(db)},
		{"POST", "/v1/feeds", loggedIn(db, handleAddFeed(db))},
		{"GET", "/v1/feeds", loggedIn(db, handleListFeeds(db))},
		{"GET", "/v1/follows", loggedIn(db, handleListFollows(db))},
		{"POST", "/v1/follows", loggedIn(db, handleCreateFollow(db))},
		{"DELETE", "/v1/follows", loggedIn(db, handleDeleteFollow(db))},
		{"GET", "/v1/posts", loggedIn(db, handleListPosts(db))},
		{"GET", "/v1/bookmarks", loggedIn(db, handleListBookmarks(db))},
		{"POST", "/v1/bookmarks", loggedIn(db, handleCreateBookmark(db))},
		{"DELETE", "/v1/bookmarks", loggedIn(db, handleDeleteBookmark(db))},
		{"GET", "/v1/search", loggedIn(db, handleSearch(db))},
	}

	mux := http.NewServeMux()
	allowed := make(map[string][]string)
	for _, route := range routes {
		mux.HandleFunc(route.method+" "+route.path, route.handler)
		allowed[route.path] = append(allowed[route.path], route.method)
	}
	// Without this the stdlib answers unmatched requests in plain text,
	// breaking the JSON error contract for anything as ordinary as a
	// typo'd path or the wrong verb. Registering "/" also takes over the
	// mux's own 405 handling, so handleUnmatched reproduces it.
	mux.HandleFunc("/", handleUnmatched(allowed))
	return mux
}

// handleUnmatched answers requests no route claimed, in the API's error
// shape: 405 with an Allow header when the path exists under other
// methods, 404 otherwise.
func handleUnmatched(allowed map[string][]string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		methods, ok := allowed[r.URL.Path]
		if !ok {
			respondError(w, http.StatusNotFound, "not found")
			return
		}
		methods = slices.Clone(methods)
		slices.Sort(methods)
		w.Header().Set("Allow", strings.Join(methods, ", "))
		respondError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// urlFromBody decodes a `{"url": ...}` request body, writing a 400 error
// response itself when the body is missing or has no url.
func urlFromBody(w http.ResponseWriter, r *http.Request) (string, bool) {
	var params struct {
		Url string `json:"url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&params); err != nil {
		respondError(w, http.StatusBadRequest, "invalid JSON body")
		return "", false
	}
	if params.Url == "" {
		respondError(w, http.StatusBadRequest, "url is required")
		return "", false
	}
	return params.Url, true
}

// limitParam reads the optional `limit` query parameter, falling back to
// def when it's absent and writing a 400 error response itself when it's
// present but not a positive integer. It parses at the int32 width the
// sqlc params take, so an out-of-range limit is a clean 400 rather than a
// value that wraps negative and reaches Postgres as `LIMIT -2147483648`.
func limitParam(w http.ResponseWriter, r *http.Request, def int32) (int32, bool) {
	raw := r.URL.Query().Get("limit")
	if raw == "" {
		return def, true
	}
	parsed, err := strconv.ParseInt(raw, 10, 32)
	if err != nil || parsed <= 0 {
		respondError(w, http.StatusBadRequest, "invalid limit")
		return 0, false
	}
	return int32(parsed), true
}

func respondJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		log.Printf("writing response: %v", err)
	}
}

func respondError(w http.ResponseWriter, status int, message string) {
	respondJSON(w, status, map[string]string{"error": message})
}
