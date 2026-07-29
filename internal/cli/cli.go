package cli

import (
	"context"
	"database/sql"
	"errors"

	"github.com/jacobrluttrull/gator/internal/config"
	"github.com/jacobrluttrull/gator/internal/database"
)

type State struct {
	Config *config.Config
	DB     *database.Queries
	// Conn is the raw connection DB wraps, for handlers (namely the API's
	// transactional endpoints) that need to start a transaction rather
	// than issue independent queries through DB.
	Conn *sql.DB
}

type Command struct {
	Name string
	Args []string
}

type Commands struct {
	Handlers map[string]func(*State, Command) error
}

func (c *Commands) Run(s *State, cmd Command) error {
	f, ok := c.Handlers[cmd.Name]
	if !ok {
		return errors.New("unknown command: " + cmd.Name)
	}
	return f(s, cmd)
}

// LoggedIn wraps a handler that requires the currently logged-in user,
// looking the user up and injecting it so the handler doesn't have to.
func LoggedIn(handler func(s *State, cmd Command, user database.User) error) func(*State, Command) error {
	return func(s *State, cmd Command) error {
		user, err := s.DB.GetUser(context.Background(), s.Config.CurrentUserName)
		if err != nil {
			return err
		}
		return handler(s, cmd, user)
	}
}
