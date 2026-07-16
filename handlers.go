package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/google/uuid"

	"github.com/jacobrluttrull/gator/internal/database"
)

func handlerLogin(s *state, cmd command) error {
	if len(cmd.args) == 0 {
		return errors.New("the login handler expects a single argument: the username")
	}
	name := cmd.args[0]

	_, err := s.db.GetUser(context.Background(), name)
	if err != nil {
		fmt.Printf("couldn't find user: %v\n", err)
		os.Exit(1)
	}

	if err := s.Config.SetUser(name); err != nil {
		return err
	}
	fmt.Printf("Username set to: %s\n", name)
	return nil
}

func registerHandler(s *state, cmd command) error {
	if len(cmd.args) == 0 {
		return errors.New("the register handler expects a single argument: the username")
	}
	name := cmd.args[0]

	user, err := s.db.CreateUser(context.Background(), database.CreateUserParams{
		ID:        uuid.New(),
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
		Name:      name,
	})
	if err != nil {
		return err
	}
	if err := s.Config.SetUser(user.Name); err != nil {
		return fmt.Errorf("could not set user %s: %s", user.Name, err)
	}
	fmt.Println("User set to: " + user.Name)
	return nil
}

func handlerReset(s *state, cmd command) error {
	err := s.db.DeleteUsers(context.Background())
	if err != nil {
		return err
	}
	fmt.Println("Users deleted")
	return nil
}

func handlerUsers(s *state, cmd command) error {
	users, err := s.db.GetUsers(context.Background())
	if err != nil {
		return err
	}
	for _, name := range users {
		if name == s.Config.CurrentUserName {
			fmt.Printf("* %s (current)\n", name)
		} else {
			fmt.Printf("* %s\n", name)
		}
	}
	return nil
}

func handlerAgg(s *state, cmd command) error {
	if len(cmd.args) != 1 {
		return errors.New("the agg handler expects a single argument: time_between_reqs")
	}
	timeBetweenRequests, err := time.ParseDuration(cmd.args[0])
	if err != nil {
		return err
	}
	fmt.Printf("Collecting feeds every %s\n", timeBetweenRequests)

	ticker := time.NewTicker(timeBetweenRequests)
	for ; ; <-ticker.C {
		scrapFeeds(s)
	}
}

func handlerAddFeed(s *state, cmd command, user database.User) error {
	if len(cmd.args) != 2 {
		return errors.New("the add feed handler expects two arguments")
	}
	name := cmd.args[0]
	url := cmd.args[1]

	feed, err := s.db.CreateFeed(context.Background(), database.CreateFeedParams{
		ID:        uuid.New(),
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
		Name:      name,
		Url:       url,
		UserID:    user.ID,
	})
	if err != nil {
		return err
	}
	fmt.Printf("%+v\n", feed)

	feedFollow, err := s.db.CreateFeedFollow(context.Background(), database.CreateFeedFollowParams{
		ID:        uuid.New(),
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
		UserID:    user.ID,
		FeedID:    feed.ID,
	})
	if err != nil {
		return err
	}
	fmt.Printf("%s is now following %s\n", feedFollow.UserName, feedFollow.FeedName)
	return nil
}

func handlerGetFeeds(s *state, cmd command) error {
	feeds, err := s.db.GetFeeds(context.Background())
	if err != nil {
		return err
	}
	for _, feed := range feeds {
		fmt.Printf("%s (%s) - added by %s\n", feed.FeedName, feed.FeedUrl, feed.Username)
	}
	return nil
}

func handlerFollow(s *state, cmd command, user database.User) error {
	if len(cmd.args) != 1 {
		return errors.New("the follow handler expects one argument")
	}
	url := cmd.args[0]

	feed, err := s.db.GetFeedByUrl(context.Background(), url)
	if err != nil {
		return err
	}

	feedFollow, err := s.db.CreateFeedFollow(context.Background(), database.CreateFeedFollowParams{
		ID:        uuid.New(),
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
		UserID:    user.ID,
		FeedID:    feed.ID,
	})
	if err != nil {
		return err
	}
	fmt.Printf("%s is now following %s\n", feedFollow.UserName, feedFollow.FeedName)
	return nil
}
func handlerFollowing(s *state, cmd command, user database.User) error {
	feedFollows, err := s.db.GetFeedFollowsForUser(context.Background(), user.ID)
	if err != nil {
		return err
	}
	for _, ff := range feedFollows {
		fmt.Println(ff.FeedName)
	}
	return nil
}
func handlerUnfollow(s *state, cmd command, user database.User) error {
	if len(cmd.args) != 1 {
		return errors.New("the unfollow handler expects one argument")
	}
	url := cmd.args[0]

	feed, err := s.db.GetFeedByUrl(context.Background(), url)

	if err != nil {
		return err
	}
	return s.db.DeleteFeedFollow(context.Background(), database.DeleteFeedFollowParams{
		UserID: user.ID,
		FeedID: feed.ID,
	})

}
func handlerBrowse(s *state, cmd command, user database.User) error {
	limit := 2
	if len(cmd.args) >= 1 {
		parsed, err := strconv.Atoi(cmd.args[0])
		if err != nil {
			return fmt.Errorf("invalid limit: %w", err)
		}
		limit = parsed
	}

	ascending := len(cmd.args) >= 2 && cmd.args[1] == "asc"

	page := 1
	if len(cmd.args) >= 3 {
		parsed, err := strconv.Atoi(cmd.args[2])
		if err != nil {
			return fmt.Errorf("invalid page: %w", err)
		}
		page = parsed
	}
	offset := (page - 1) * limit
	var posts []database.Post
	var err error
	if ascending {
		posts, err = s.db.GetPostsForUserAsc(context.Background(), database.GetPostsForUserAscParams{
			UserID: user.ID,
			Limit:  int32(limit),
			Offset: int32(offset),
		})
	} else {
		posts, err = s.db.GetPostsForUser(context.Background(), database.GetPostsForUserParams{
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

func handlerBookmark(s *state, cmd command, user database.User) error {
	if len(cmd.args) != 1 {
		return errors.New("the bookmark handler expects one argument: the post url")
	}
	post, err := s.db.GetPostByUrl(context.Background(), cmd.args[0])
	if err != nil {
		return err
	}
	bookmark, err := s.db.CreateBookmark(context.Background(), database.CreateBookmarkParams{
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

func handlerUnbookmark(s *state, cmd command, user database.User) error {
	if len(cmd.args) != 1 {
		return errors.New("the unbookmark handler expects one argument: the post url")
	}
	post, err := s.db.GetPostByUrl(context.Background(), cmd.args[0])
	if err != nil {
		return err
	}
	return s.db.DeleteBookmark(context.Background(), database.DeleteBookmarkParams{
		UserID: user.ID,
		PostID: post.ID,
	})
}

func handlerBookmarks(s *state, cmd command, user database.User) error {
	posts, err := s.db.GetBookmarksForUser(context.Background(), user.ID)
	if err != nil {
		return err
	}
	for _, post := range posts {
		fmt.Printf("%s\n%s\n\n", post.Title, post.Url)
	}
	return nil
}

func handlerSearchPosts(s *state, cmd command, user database.User) error {
	if len(cmd.args) < 1 {
		return errors.New("the search handler expects at least one argument: the search term")
	}
	term := cmd.args[0]

	limit := 5
	if len(cmd.args) >= 2 {
		parsed, err := strconv.Atoi(cmd.args[1])
		if err != nil {
			return fmt.Errorf("invalid limit: %w", err)
		}
		limit = parsed
	}

	posts, err := s.db.SearchPosts(context.Background(), database.SearchPostsParams{
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
