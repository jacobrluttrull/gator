package api

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"

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
	return mux
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
