package api

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/jacobrluttrull/gator/internal/auth"
	"github.com/jacobrluttrull/gator/internal/database"
)

// User is the API view of a user row: the sqlc row minus password material.
type User struct {
	ID        uuid.UUID `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Name      string    `json:"name"`
}

func apiUser(u database.User) User {
	return User{
		ID:        u.ID,
		CreatedAt: u.CreatedAt,
		UpdatedAt: u.UpdatedAt,
		Name:      u.Name,
	}
}

func handleRegister(db *database.Queries) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var params struct {
			Name     string `json:"name"`
			Password string `json:"password"`
		}
		if err := json.NewDecoder(r.Body).Decode(&params); err != nil {
			respondError(w, http.StatusBadRequest, "invalid JSON body")
			return
		}
		if params.Name == "" || params.Password == "" {
			respondError(w, http.StatusBadRequest, "name and password are required")
			return
		}

		hash, err := auth.HashPassword(params.Password)
		if err != nil {
			respondError(w, http.StatusInternalServerError, "couldn't hash password")
			return
		}

		now := time.Now().UTC()
		user, err := db.CreateUserWithPassword(r.Context(), database.CreateUserWithPasswordParams{
			ID:           uuid.New(),
			CreatedAt:    now,
			UpdatedAt:    now,
			Name:         params.Name,
			PasswordHash: sql.NullString{String: hash, Valid: true},
		})
		if err != nil {
			if isUniqueViolation(err) {
				respondError(w, http.StatusConflict, "a user with that name already exists")
				return
			}
			respondError(w, http.StatusInternalServerError, "couldn't create user")
			return
		}

		respondJSON(w, http.StatusCreated, apiUser(user))
	}
}
