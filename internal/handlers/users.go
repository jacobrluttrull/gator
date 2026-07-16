package handlers

import (
	"context"
	"fmt"

	"github.com/jacobrluttrull/gator/internal/cli"
)

func Reset(s *cli.State, cmd cli.Command) error {
	err := s.DB.DeleteUsers(context.Background())
	if err != nil {
		return err
	}
	fmt.Println("Users deleted")
	return nil
}

func Users(s *cli.State, cmd cli.Command) error {
	users, err := s.DB.GetUsers(context.Background())
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
