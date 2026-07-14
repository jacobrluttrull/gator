package main

import (
	"database/sql"
	"gator/internal/database"

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
	if err := s.Config.SetUser(cmd.args[0]); err != nil {
		return err
	}
	fmt.Printf("Username set to: %s\n", s.Config.CurrentUserName)
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
	fmt.Printf("Config before update: %+v\n", cfg)

	appState := &state{Config: &cfg}

	cmds := commands{
		commands: make(map[string]func(*state, command) error),
	}
	cmds.register("login", handlerLogin)

	if len(os.Args) < 2 {
		log.Fatal("usage: cli <command> [args...]")
	}

	err = cmds.run(appState, command{name: os.Args[1], args: os.Args[2:]})
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Config after update: %+v\n", cfg)
}
