package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/jacobrluttrull/gator/internal/auth"
	"github.com/jacobrluttrull/gator/internal/database"
)

func handleLogin(db *database.Queries) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var params struct {
			Name     string `json:"name"`
			Password string `json:"password"`
		}
		if err := json.NewDecoder(r.Body).Decode(&params); err != nil {
			respondError(w, http.StatusBadRequest, "invalid JSON body")
			return
		}

		// Missing or empty credentials are not a validation error but a
		// failed login: they fall through the checks below to a 401, so a
		// CLI-only user's null hash can never be entered with an empty
		// password.
		user, err := db.GetUser(r.Context(), params.Name)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				respondError(w, http.StatusUnauthorized, "invalid username or password")
				return
			}
			respondError(w, http.StatusInternalServerError, "couldn't look up user")
			return
		}
		// A CLI-only user (null password hash) can never log in over the
		// network until they set a password from the trusted CLI.
		if !user.PasswordHash.Valid {
			respondError(w, http.StatusUnauthorized, "invalid username or password")
			return
		}
		if err := auth.CheckPasswordHash(params.Password, user.PasswordHash.String); err != nil {
			respondError(w, http.StatusUnauthorized, "invalid username or password")
			return
		}

		key, err := auth.GenerateAPIKey()
		if err != nil {
			respondError(w, http.StatusInternalServerError, "couldn't generate API key")
			return
		}
		now := time.Now().UTC()
		if _, err := db.CreateAPIKey(r.Context(), database.CreateAPIKeyParams{
			ID:        uuid.New(),
			CreatedAt: now,
			UpdatedAt: now,
			KeyHash:   auth.HashAPIKey(key),
			UserID:    user.ID,
		}); err != nil {
			respondError(w, http.StatusInternalServerError, "couldn't store API key")
			return
		}

		respondJSON(w, http.StatusOK, map[string]string{"api_key": key})
	}
}
