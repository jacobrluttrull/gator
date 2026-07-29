package api

import (
	"net/http"
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

// postFromRow converts a posts-table row into its API view.
func postFromRow(row database.Post) Post {
	return Post{
		ID:          row.ID,
		CreatedAt:   row.CreatedAt,
		UpdatedAt:   row.UpdatedAt,
		Title:       row.Title,
		Url:         row.Url,
		Description: row.Description,
		PublishedAt: row.PublishedAt,
		FeedID:      row.FeedID,
	}
}

// postsFromRows converts posts-table rows into their API views. The result
// is never nil, so an empty list encodes as [] rather than null.
func postsFromRows(rows []database.Post) []Post {
	posts := make([]Post, 0, len(rows))
	for _, row := range rows {
		posts = append(posts, postFromRow(row))
	}
	return posts
}

func handleListPosts(db *database.Queries) authedHandler {
	return func(w http.ResponseWriter, r *http.Request, user database.User) {
		limit, ok := limitParam(w, r, defaultPostsLimit)
		if !ok {
			return
		}
		rows, err := db.GetPostsForUser(r.Context(), database.GetPostsForUserParams{
			UserID: user.ID,
			Limit:  limit,
			Offset: 0,
		})
		if err != nil {
			respondError(w, http.StatusInternalServerError, "couldn't list posts")
			return
		}
		respondJSON(w, http.StatusOK, postsFromRows(rows))
	}
}
