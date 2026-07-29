package api

import (
	"net/http"
	"strings"

	"github.com/jacobrluttrull/gator/internal/database"
)

// defaultSearchLimit matches the CLI search command's default limit.
const defaultSearchLimit = 5

// SearchResult is the API view of a search hit: the SearchPosts sqlc row as
// JSON — a post plus its trigram similarity to the query.
type SearchResult struct {
	Post
	Sim float32 `json:"sim"`
}

// handleSearch fuzzy-matches post titles among the caller's Followings,
// mirroring the CLI's search command.
func handleSearch(db *database.Queries) authedHandler {
	return func(w http.ResponseWriter, r *http.Request, user database.User) {
		query := strings.TrimSpace(r.URL.Query().Get("q"))
		if query == "" {
			respondError(w, http.StatusBadRequest, "q is required")
			return
		}
		limit, ok := limitParam(w, r, defaultSearchLimit)
		if !ok {
			return
		}

		rows, err := db.SearchPosts(r.Context(), database.SearchPostsParams{
			UserID: user.ID,
			Lower:  query,
			Limit:  int32(limit),
		})
		if err != nil {
			respondError(w, http.StatusInternalServerError, "couldn't search posts")
			return
		}

		results := make([]SearchResult, 0, len(rows))
		for _, row := range rows {
			results = append(results, SearchResult{
				Post: Post{
					ID:          row.ID,
					CreatedAt:   row.CreatedAt,
					UpdatedAt:   row.UpdatedAt,
					Title:       row.Title,
					Url:         row.Url,
					Description: row.Description,
					PublishedAt: row.PublishedAt,
					FeedID:      row.FeedID,
				},
				Sim: row.Sim,
			})
		}
		respondJSON(w, http.StatusOK, results)
	}
}
