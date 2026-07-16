package handlers

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/google/uuid"

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
