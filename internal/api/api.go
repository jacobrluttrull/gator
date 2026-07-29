package api

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"

	"github.com/lib/pq"

	"github.com/jacobrluttrull/gator/internal/database"
)

func isUniqueViolation(err error) bool {
	var pqErr *pq.Error
	return errors.As(err, &pqErr) && pqErr.Code.Name() == "unique_violation"
}

// New returns the /v1 API handler backed by the given DB layer.
func New(db *database.Queries) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/register", handleRegister(db))
	mux.HandleFunc("POST /v1/login", handleLogin(db))
	mux.HandleFunc("POST /v1/feeds", loggedIn(db, handleAddFeed(db)))
	mux.HandleFunc("GET /v1/feeds", loggedIn(db, handleListFeeds(db)))
	mux.HandleFunc("GET /v1/follows", loggedIn(db, handleListFollows(db)))
	mux.HandleFunc("POST /v1/follows", loggedIn(db, handleCreateFollow(db)))
	mux.HandleFunc("DELETE /v1/follows", loggedIn(db, handleDeleteFollow(db)))
	mux.HandleFunc("GET /v1/posts", loggedIn(db, handleListPosts(db)))
	mux.HandleFunc("GET /v1/bookmarks", loggedIn(db, handleListBookmarks(db)))
	mux.HandleFunc("POST /v1/bookmarks", loggedIn(db, handleCreateBookmark(db)))
	mux.HandleFunc("DELETE /v1/bookmarks", loggedIn(db, handleDeleteBookmark(db)))
	mux.HandleFunc("GET /v1/search", loggedIn(db, handleSearch(db)))
	return mux
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
