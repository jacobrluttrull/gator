package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/google/uuid"

	"gator/internal/database"
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
	feed, err := fetchFeed(context.Background(), "https://www.wagslane.dev/index.xml")
	if err != nil {
		return err
	}
	fmt.Printf("%+v\n", feed)
	return nil
}

func handlerAddFeed(s *state, cmd command) error {
	if len(cmd.args) != 2 {
		return errors.New("the add feed handler expects two arguments")
	}
	name := cmd.args[0]
	url := cmd.args[1]

	user, err := s.db.GetUser(context.Background(), s.Config.CurrentUserName)
	if err != nil {
		return err
	}
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

func handlerFollow(s *state, cmd command) error {
	if len(cmd.args) != 1 {
		return errors.New("the follow handler expects one argument")
	}
	url := cmd.args[0]

	user, err := s.db.GetUser(context.Background(), s.Config.CurrentUserName)
	if err != nil {
		return err
	}
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
	fmt.Printf("%s is now following%s\n", feedFollow.UserName, feedFollow.FeedName)
	return nil
}
func handlerFollowing(s *state, cmd command) error {
	user, err := s.db.GetUser(context.Background(), s.Config.CurrentUserName)
	if err != nil {
		return err
	}
	feedFollows, err := s.db.GetFeedFollowsForUser(context.Background(), user.ID)
	if err != nil {
		return err
	}
	for _, ff := range feedFollows {
		fmt.Println(ff.FeedName)
	}
	return nil
}
