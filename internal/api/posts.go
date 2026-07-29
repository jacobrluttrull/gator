package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"

	"github.com/jacobrluttrull/gator/internal/database"
)

// defaultPostsLimit matches the CLI browse command's default limit.
const defaultPostsLimit = 2

// Post is the API view of a post: the GetPostsForUser sqlc row as JSON.
type Post struct {
	ID          uuid.UUID `json:"id"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	Title       string    `json:"title"`
	Url         string    `json:"url"`
	Description string    `json:"description"`
	PublishedAt time.Time `json:"published_at"`
	FeedID      uuid.UUID `json:"feed_id"`
}

func handleListPosts(db *database.Queries) authedHandler {
	return func(w http.ResponseWriter, r *http.Request, user database.User) {
		limit := defaultPostsLimit
		if raw := r.URL.Query().Get("limit"); raw != "" {
			parsed, err := strconv.Atoi(raw)
			if err != nil || parsed <= 0 {
				respondError(w, http.StatusBadRequest, "invalid limit")
				return
			}
			limit = parsed
		}
		rows, err := db.GetPostsForUser(r.Context(), database.GetPostsForUserParams{
			UserID: user.ID,
			Limit:  int32(limit),
			Offset: 0,
		})
		if err != nil {
			respondError(w, http.StatusInternalServerError, "couldn't list posts")
			return
		}
		posts := make([]Post, 0, len(rows))
		for _, row := range rows {
			posts = append(posts, Post{
				ID:          row.ID,
				CreatedAt:   row.CreatedAt,
				UpdatedAt:   row.UpdatedAt,
				Title:       row.Title,
				Url:         row.Url,
				Description: row.Description,
				PublishedAt: row.PublishedAt,
				FeedID:      row.FeedID,
			})
		}
		respondJSON(w, http.StatusOK, posts)
	}
}
