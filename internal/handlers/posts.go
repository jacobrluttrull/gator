package handlers

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/google/uuid"

	"github.com/jacobrluttrull/gator/internal/cli"
	"github.com/jacobrluttrull/gator/internal/database"
)

func Browse(s *cli.State, cmd cli.Command, user database.User) error {
	limit := 2
	if len(cmd.Args) >= 1 {
		parsed, err := strconv.Atoi(cmd.Args[0])
		if err != nil {
			return fmt.Errorf("invalid limit: %w", err)
		}
		limit = parsed
	}

	ascending := len(cmd.Args) >= 2 && cmd.Args[1] == "asc"

	page := 1
	if len(cmd.Args) >= 3 {
		parsed, err := strconv.Atoi(cmd.Args[2])
		if err != nil {
			return fmt.Errorf("invalid page: %w", err)
		}
		page = parsed
	}
	offset := (page - 1) * limit
	var posts []database.Post
	var err error
	if ascending {
		posts, err = s.DB.GetPostsForUserAsc(context.Background(), database.GetPostsForUserAscParams{
			UserID: user.ID,
			Limit:  int32(limit),
			Offset: int32(offset),
		})
	} else {
		posts, err = s.DB.GetPostsForUser(context.Background(), database.GetPostsForUserParams{
			UserID: user.ID,
			Limit:  int32(limit),
			Offset: int32(offset),
		})
	}
	if err != nil {
		return err
	}

	for _, post := range posts {
		fmt.Printf("%s\n%s\n\n", post.Title, post.Url)
	}
	return nil
}

func Bookmark(s *cli.State, cmd cli.Command, user database.User) error {
	if len(cmd.Args) != 1 {
		return errors.New("the bookmark handler expects one argument: the post url")
	}
	post, err := s.DB.GetPostByUrl(context.Background(), cmd.Args[0])
	if err != nil {
		return err
	}
	bookmark, err := s.DB.CreateBookmark(context.Background(), database.CreateBookmarkParams{
		ID:        uuid.New(),
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
		UserID:    user.ID,
		PostID:    post.ID,
	})
	if err != nil {
		return err
	}
	fmt.Printf("Bookmarked: %s\n", bookmark.PostTitle)
	return nil
}

func Unbookmark(s *cli.State, cmd cli.Command, user database.User) error {
	if len(cmd.Args) != 1 {
		return errors.New("the unbookmark handler expects one argument: the post url")
	}
	post, err := s.DB.GetPostByUrl(context.Background(), cmd.Args[0])
	if err != nil {
		return err
	}
	return s.DB.DeleteBookmark(context.Background(), database.DeleteBookmarkParams{
		UserID: user.ID,
		PostID: post.ID,
	})
}

func Bookmarks(s *cli.State, cmd cli.Command, user database.User) error {
	posts, err := s.DB.GetBookmarksForUser(context.Background(), user.ID)
	if err != nil {
		return err
	}
	for _, post := range posts {
		fmt.Printf("%s\n%s\n\n", post.Title, post.Url)
	}
	return nil
}

func Search(s *cli.State, cmd cli.Command, user database.User) error {
	if len(cmd.Args) < 1 {
		return errors.New("the search handler expects at least one argument: the search term")
	}
	term := cmd.Args[0]

	limit := 5
	if len(cmd.Args) >= 2 {
		parsed, err := strconv.Atoi(cmd.Args[1])
		if err != nil {
			return fmt.Errorf("invalid limit: %w", err)
		}
		limit = parsed
	}

	posts, err := s.DB.SearchPosts(context.Background(), database.SearchPostsParams{
		UserID: user.ID,
		Lower:  term,
		Limit:  int32(limit),
	})
	if err != nil {
		return err
	}

	for _, post := range posts {
		fmt.Printf("%s (%.2f match)\n%s\n\n", post.Title, post.Sim, post.Url)
	}
	return nil
}
