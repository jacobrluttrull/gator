package main

import (
	"context"
	"database/sql"
	"gator/internal/database"
	"time"

	"github.com/google/uuid"
	_ "github.com/lib/pq"
)

import (
	"errors"
	"fmt"
	"log"
	"os"

	"gator/internal/config"
)

type state struct {
	Config *config.Config
	db     *database.Queries
}

type command struct {
	name string
	args []string
}

type commands struct {
	commands map[string]func(*state, command) error
}

func (c *commands) run(s *state, cmd command) error {
	f, ok := c.commands[cmd.name]
	if !ok {
		return errors.New("unknown command: " + cmd.name)
	}
	return f(s, cmd)
}

func (c *commands) register(name string, f func(*state, command) error) {
	c.commands[name] = f
}

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
		os.Exit(1)
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

func main() {
	cfg, err := config.Read()
	if err != nil {
		log.Fatal(err)
	}
	db, err := sql.Open("postgres", cfg.DBURL)
	if err != nil {
		log.Fatal(err)
	}
	dbQueries := database.New(db)

	appState := &state{
		Config: &cfg,
		db:     dbQueries,
	}

	cmds := commands{
		commands: make(map[string]func(*state, command) error),
	}
	cmds.register("login", handlerLogin)
	cmds.register("register", registerHandler)
	cmds.register("reset", handlerReset)
	cmds.register("users", handlerUsers)
	cmds.register("agg", handlerAgg)
	cmds.register("addfeed", handlerAddFeed)
	cmds.register("feeds", handlerGetFeeds)

	if len(os.Args) < 2 {
		log.Fatal("usage: cli <command> [args...]")
	}

	err = cmds.run(appState, command{name: os.Args[1], args: os.Args[2:]})
	if err != nil {
		log.Fatal(err)
	}
}
