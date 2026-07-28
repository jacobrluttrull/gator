package api

import (
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/jacobrluttrull/gator/internal/database"
)

// FeedFollow is the API view of a follow: the GetFeedFollowsForUser
// sqlc row as JSON.
type FeedFollow struct {
	ID        uuid.UUID `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	UserID    uuid.UUID `json:"user_id"`
	FeedID    uuid.UUID `json:"feed_id"`
	UserName  string    `json:"user_name"`
	FeedName  string    `json:"feed_name"`
}

func handleListFollows(db *database.Queries) authedHandler {
	return func(w http.ResponseWriter, r *http.Request, user database.User) {
		rows, err := db.GetFeedFollowsForUser(r.Context(), user.ID)
		if err != nil {
			respondError(w, http.StatusInternalServerError, "couldn't list follows")
			return
		}
		follows := make([]FeedFollow, 0, len(rows))
		for _, row := range rows {
			follows = append(follows, FeedFollow{
				ID:        row.ID,
				CreatedAt: row.CreatedAt,
				UpdatedAt: row.UpdatedAt,
				UserID:    row.UserID,
				FeedID:    row.FeedID,
				UserName:  row.UserName,
				FeedName:  row.FeedName,
			})
		}
		respondJSON(w, http.StatusOK, follows)
	}
}
