package handlers

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/jacobrluttrull/gator/internal/cli"
	"github.com/jacobrluttrull/gator/internal/database"
)

func AddFeed(s *cli.State, cmd cli.Command, user database.User) error {
	if len(cmd.Args) != 2 {
		return errors.New("the add feed handler expects two arguments")
	}
	name := cmd.Args[0]
	url := cmd.Args[1]

	feed, err := s.DB.CreateFeed(context.Background(), database.CreateFeedParams{
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

	feedFollow, err := s.DB.CreateFeedFollow(context.Background(), database.CreateFeedFollowParams{
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

func Feeds(s *cli.State, cmd cli.Command) error {
	feeds, err := s.DB.GetFeeds(context.Background())
	if err != nil {
		return err
	}
	for _, feed := range feeds {
		fmt.Printf("%s (%s) - added by %s\n", feed.FeedName, feed.FeedUrl, feed.Username)
	}
	return nil
}

func Follow(s *cli.State, cmd cli.Command, user database.User) error {
	if len(cmd.Args) != 1 {
		return errors.New("the follow handler expects one argument")
	}
	url := cmd.Args[0]

	feed, err := s.DB.GetFeedByUrl(context.Background(), url)
	if err != nil {
		return err
	}

	feedFollow, err := s.DB.CreateFeedFollow(context.Background(), database.CreateFeedFollowParams{
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

func Following(s *cli.State, cmd cli.Command, user database.User) error {
	feedFollows, err := s.DB.GetFeedFollowsForUser(context.Background(), user.ID)
	if err != nil {
		return err
	}
	for _, ff := range feedFollows {
		fmt.Println(ff.FeedName)
	}
	return nil
}

func Unfollow(s *cli.State, cmd cli.Command, user database.User) error {
	if len(cmd.Args) != 1 {
		return errors.New("the unfollow handler expects one argument")
	}
	url := cmd.Args[0]

	feed, err := s.DB.GetFeedByUrl(context.Background(), url)
	if err != nil {
		return err
	}
	return s.DB.DeleteFeedFollow(context.Background(), database.DeleteFeedFollowParams{
		UserID: user.ID,
		FeedID: feed.ID,
	})
}
