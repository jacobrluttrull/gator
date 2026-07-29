package api

import (
	"database/sql"
	"encoding/json"
	"errors"
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

// feedFromURLBody decodes a `{"url": ...}` request body and resolves it
// to a feed, writing the appropriate error response (400 on a bad body,
// 404 on an unknown URL) itself when it can't.
func feedFromURLBody(db *database.Queries, w http.ResponseWriter, r *http.Request) (database.Feed, bool) {
	var params struct {
		Url string `json:"url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&params); err != nil {
		respondError(w, http.StatusBadRequest, "invalid JSON body")
		return database.Feed{}, false
	}
	if params.Url == "" {
		respondError(w, http.StatusBadRequest, "url is required")
		return database.Feed{}, false
	}
	feed, err := db.GetFeedByUrl(r.Context(), params.Url)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			respondError(w, http.StatusNotFound, "no feed with that url")
			return database.Feed{}, false
		}
		respondError(w, http.StatusInternalServerError, "couldn't look up feed")
		return database.Feed{}, false
	}
	return feed, true
}

// handleCreateFollow follows an existing shared-pool feed by URL,
// mirroring the CLI's follow command.
func handleCreateFollow(db *database.Queries) authedHandler {
	return func(w http.ResponseWriter, r *http.Request, user database.User) {
		feed, ok := feedFromURLBody(db, w, r)
		if !ok {
			return
		}

		now := time.Now().UTC()
		follow, err := db.CreateFeedFollow(r.Context(), database.CreateFeedFollowParams{
			ID:        uuid.New(),
			CreatedAt: now,
			UpdatedAt: now,
			UserID:    user.ID,
			FeedID:    feed.ID,
		})
		if err != nil {
			if isUniqueViolation(err) {
				respondError(w, http.StatusConflict, "already following that feed")
				return
			}
			respondError(w, http.StatusInternalServerError, "couldn't follow feed")
			return
		}

		respondJSON(w, http.StatusCreated, FeedFollow{
			ID:        follow.ID,
			CreatedAt: follow.CreatedAt,
			UpdatedAt: follow.UpdatedAt,
			UserID:    follow.UserID,
			FeedID:    follow.FeedID,
			UserName:  follow.UserName,
			FeedName:  follow.FeedName,
		})
	}
}

// handleDeleteFollow unfollows a feed by URL, mirroring the CLI's
// unfollow command.
func handleDeleteFollow(db *database.Queries) authedHandler {
	return func(w http.ResponseWriter, r *http.Request, user database.User) {
		feed, ok := feedFromURLBody(db, w, r)
		if !ok {
			return
		}

		if err := db.DeleteFeedFollow(r.Context(), database.DeleteFeedFollowParams{
			UserID: user.ID,
			FeedID: feed.ID,
		}); err != nil {
			respondError(w, http.StatusInternalServerError, "couldn't unfollow feed")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
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
