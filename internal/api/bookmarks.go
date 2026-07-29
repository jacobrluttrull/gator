package api

import (
	"database/sql"
	"errors"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/jacobrluttrull/gator/internal/database"
)

// Bookmark is the API view of a bookmark: the CreateBookmark sqlc row as
// JSON. Bookmarks are per-user — a user only ever sees their own.
type Bookmark struct {
	ID        uuid.UUID `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	UserID    uuid.UUID `json:"user_id"`
	PostID    uuid.UUID `json:"post_id"`
	PostTitle string    `json:"post_title"`
	PostUrl   string    `json:"post_url"`
}

// postFromURLBody decodes a `{"url": ...}` request body and resolves it to
// a post, writing the appropriate error response (400 on a bad body, 404
// on an unknown URL) itself when it can't.
func postFromURLBody(db *database.Queries, w http.ResponseWriter, r *http.Request) (database.Post, bool) {
	url, ok := urlFromBody(w, r)
	if !ok {
		return database.Post{}, false
	}
	post, err := db.GetPostByUrl(r.Context(), url)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			respondError(w, http.StatusNotFound, "no post with that url")
			return database.Post{}, false
		}
		respondError(w, http.StatusInternalServerError, "couldn't look up post")
		return database.Post{}, false
	}
	return post, true
}

// handleCreateBookmark bookmarks a post by URL, mirroring the CLI's
// bookmark command.
func handleCreateBookmark(db *database.Queries) authedHandler {
	return func(w http.ResponseWriter, r *http.Request, user database.User) {
		post, ok := postFromURLBody(db, w, r)
		if !ok {
			return
		}

		now := time.Now().UTC()
		bookmark, err := db.CreateBookmark(r.Context(), database.CreateBookmarkParams{
			ID:        uuid.New(),
			CreatedAt: now,
			UpdatedAt: now,
			UserID:    user.ID,
			PostID:    post.ID,
		})
		if err != nil {
			if isUniqueViolation(err) {
				respondError(w, http.StatusConflict, "already bookmarked that post")
				return
			}
			respondError(w, http.StatusInternalServerError, "couldn't bookmark post")
			return
		}

		respondJSON(w, http.StatusCreated, Bookmark{
			ID:        bookmark.ID,
			CreatedAt: bookmark.CreatedAt,
			UpdatedAt: bookmark.UpdatedAt,
			UserID:    bookmark.UserID,
			PostID:    bookmark.PostID,
			PostTitle: bookmark.PostTitle,
			PostUrl:   bookmark.PostUrl,
		})
	}
}

// handleDeleteBookmark removes the caller's bookmark of a post by URL,
// mirroring the CLI's unbookmark command.
func handleDeleteBookmark(db *database.Queries) authedHandler {
	return func(w http.ResponseWriter, r *http.Request, user database.User) {
		post, ok := postFromURLBody(db, w, r)
		if !ok {
			return
		}

		if err := db.DeleteBookmark(r.Context(), database.DeleteBookmarkParams{
			UserID: user.ID,
			PostID: post.ID,
		}); err != nil {
			respondError(w, http.StatusInternalServerError, "couldn't unbookmark post")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// handleListBookmarks lists the caller's bookmarked posts, newest bookmark
// first.
func handleListBookmarks(db *database.Queries) authedHandler {
	return func(w http.ResponseWriter, r *http.Request, user database.User) {
		rows, err := db.GetBookmarksForUser(r.Context(), user.ID)
		if err != nil {
			respondError(w, http.StatusInternalServerError, "couldn't list bookmarks")
			return
		}
		respondJSON(w, http.StatusOK, postsFromRows(rows))
	}
}
