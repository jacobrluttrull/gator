package handlers

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/google/uuid"

	"github.com/jacobrluttrull/gator/internal/auth"
	"github.com/jacobrluttrull/gator/internal/cli"
	"github.com/jacobrluttrull/gator/internal/database"
)

func Login(s *cli.State, cmd cli.Command) error {
	if len(cmd.Args) == 0 {
		return errors.New("the login handler expects a single argument: the username")
	}
	name := cmd.Args[0]

	_, err := s.DB.GetUser(context.Background(), name)
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

// SetPassword sets (or replaces) the current CLI user's password so they
// can log in over the API. There is no old-password check: the CLI is the
// trusted surface (ADR-0001).
func SetPassword(s *cli.State, cmd cli.Command, user database.User) error {
	if len(cmd.Args) == 0 {
		return errors.New("the setpassword handler expects a single argument: the new password")
	}
	password := cmd.Args[0]
	// An empty password would hash to a valid bcrypt entry that an empty
	// API-login could then match — keep "empty credentials never log in".
	if password == "" {
		return errors.New("password must not be empty")
	}

	hash, err := auth.HashPassword(password)
	if err != nil {
		return fmt.Errorf("could not hash password: %w", err)
	}
	if err := s.DB.SetUserPassword(context.Background(), database.SetUserPasswordParams{
		ID:           user.ID,
		PasswordHash: sql.NullString{String: hash, Valid: true},
		UpdatedAt:    time.Now().UTC(),
	}); err != nil {
		return fmt.Errorf("could not set password: %w", err)
	}
	fmt.Printf("Password set for %s\n", user.Name)
	return nil
}

func Register(s *cli.State, cmd cli.Command) error {
	if len(cmd.Args) == 0 {
		return errors.New("the register handler expects a single argument: the username")
	}
	name := cmd.Args[0]

	user, err := s.DB.CreateUser(context.Background(), database.CreateUserParams{
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
