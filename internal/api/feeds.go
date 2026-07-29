package api

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/jacobrluttrull/gator/internal/database"
)

// Feed is the API view of a feed row: the sqlc row as JSON.
type Feed struct {
	ID            uuid.UUID  `json:"id"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
	Name          string     `json:"name"`
	Url           string     `json:"url"`
	UserID        uuid.UUID  `json:"user_id"`
	LastFetchedAt *time.Time `json:"last_fetched_at"`
}

func apiFeed(f database.Feed) Feed {
	feed := Feed{
		ID:        f.ID,
		CreatedAt: f.CreatedAt,
		UpdatedAt: f.UpdatedAt,
		Name:      f.Name,
		Url:       f.Url,
		UserID:    f.UserID,
	}
	if f.LastFetchedAt.Valid {
		feed.LastFetchedAt = &f.LastFetchedAt.Time
	}
	return feed
}

// FeedListing is the API view of a shared-pool entry: the GetFeeds sqlc
// row (feed name/url plus the adder's username) as JSON.
type FeedListing struct {
	FeedName string `json:"feed_name"`
	FeedUrl  string `json:"feed_url"`
	Username string `json:"username"`
}

func handleListFeeds(db *database.Queries) authedHandler {
	return func(w http.ResponseWriter, r *http.Request, _ database.User) {
		rows, err := db.GetFeeds(r.Context())
		if err != nil {
			respondError(w, http.StatusInternalServerError, "couldn't list feeds")
			return
		}
		feeds := make([]FeedListing, 0, len(rows))
		for _, row := range rows {
			feeds = append(feeds, FeedListing{
				FeedName: row.FeedName,
				FeedUrl:  row.FeedUrl,
				Username: row.Username,
			})
		}
		respondJSON(w, http.StatusOK, feeds)
	}
}

// handleAddFeed adds a feed to the shared pool and creates the adder's
// follow in one step, mirroring the CLI's addFeed. The two writes run in
// a single transaction — otherwise a failed CreateFeedFollow leaves an
// orphaned feed behind, and a retry with the same url then fails on the
// unique constraint instead of just creating the follow.
func handleAddFeed(db *database.Queries, conn *sql.DB) authedHandler {
	return func(w http.ResponseWriter, r *http.Request, user database.User) {
		var params struct {
			Name string `json:"name"`
			Url  string `json:"url"`
		}
		if err := json.NewDecoder(r.Body).Decode(&params); err != nil {
			respondError(w, http.StatusBadRequest, "invalid JSON body")
			return
		}
		if params.Name == "" || params.Url == "" {
			respondError(w, http.StatusBadRequest, "name and url are required")
			return
		}

		tx, err := conn.BeginTx(r.Context(), nil)
		if err != nil {
			respondError(w, http.StatusInternalServerError, "couldn't create feed")
			return
		}
		defer func() { _ = tx.Rollback() }()
		qtx := db.WithTx(tx)

		now := time.Now().UTC()
		feed, err := qtx.CreateFeed(r.Context(), database.CreateFeedParams{
			ID:        uuid.New(),
			CreatedAt: now,
			UpdatedAt: now,
			Name:      params.Name,
			Url:       params.Url,
			UserID:    user.ID,
		})
		if err != nil {
			if isUniqueViolation(err) {
				respondError(w, http.StatusConflict, "a feed with that url already exists")
				return
			}
			respondError(w, http.StatusInternalServerError, "couldn't create feed")
			return
		}

		follow, err := qtx.CreateFeedFollow(r.Context(), database.CreateFeedFollowParams{
			ID:        uuid.New(),
			CreatedAt: now,
			UpdatedAt: now,
			UserID:    user.ID,
			FeedID:    feed.ID,
		})
		if err != nil {
			respondError(w, http.StatusInternalServerError, "couldn't follow feed")
			return
		}

		if err := tx.Commit(); err != nil {
			respondError(w, http.StatusInternalServerError, "couldn't create feed")
			return
		}

		respondJSON(w, http.StatusCreated, struct {
			Feed       Feed       `json:"feed"`
			FeedFollow FeedFollow `json:"feed_follow"`
		}{
			Feed: apiFeed(feed),
			FeedFollow: FeedFollow{
				ID:        follow.ID,
				CreatedAt: follow.CreatedAt,
				UpdatedAt: follow.UpdatedAt,
				UserID:    follow.UserID,
				FeedID:    follow.FeedID,
				UserName:  follow.UserName,
				FeedName:  follow.FeedName,
			},
		})
	}
}
