package api

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"

	"github.com/jacobrluttrull/gator/internal/auth"
	"github.com/jacobrluttrull/gator/internal/database"
)

// authedHandler is the HTTP analogue of the CLI's logged-in command
// handler: the middleware resolves the authenticated user and injects
// them, so handlers never look the user up themselves.
type authedHandler func(w http.ResponseWriter, r *http.Request, user database.User)

// loggedIn authenticates a request from its "Authorization: ApiKey <key>"
// header and injects the resolved user. The surface fails closed:
// missing, malformed, and unknown keys all get a uniform 401.
func loggedIn(db *database.Queries, handler authedHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		key, ok := apiKeyFromHeader(r.Header.Get("Authorization"))
		if !ok {
			respondError(w, http.StatusUnauthorized, "missing or invalid API key")
			return
		}
		user, err := db.GetUserByAPIKey(r.Context(), auth.HashAPIKey(key))
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				respondError(w, http.StatusUnauthorized, "missing or invalid API key")
				return
			}
			respondError(w, http.StatusInternalServerError, "couldn't look up user")
			return
		}
		handler(w, r, user)
	}
}

// apiKeyFromHeader extracts the key from an "ApiKey <key>" Authorization
// header value: exactly two space-separated tokens, exact scheme match.
func apiKeyFromHeader(header string) (string, bool) {
	scheme, key, found := strings.Cut(header, " ")
	if !found || scheme != "ApiKey" || key == "" || strings.ContainsRune(key, ' ') {
		return "", false
	}
	return key, true
}
