package api

import (
	"net/http"

	"github.com/jacobrluttrull/gator/internal/database"
)

// handleRevokeKeys revokes every API key the caller holds, including the
// one that authenticated this request — the whole point is that a user who
// thinks a key has leaked can kill it without knowing which one it is.
// ADR-0001 makes keys long-lived with no expiry, so this is the only way
// one ever stops working.
func handleRevokeKeys(db *database.Queries) authedHandler {
	return func(w http.ResponseWriter, r *http.Request, user database.User) {
		if err := db.DeleteAPIKeysForUser(r.Context(), user.ID); err != nil {
			respondError(w, http.StatusInternalServerError, "couldn't revoke API keys")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}
