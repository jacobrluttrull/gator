package main

import (
	"fmt"
	"gator/internal/config"
	"log"
)

func main() {
	cfg, err := config.Read()
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Config before update: %+v\n", cfg)

	appState := &state{Config: &cfg}

	cmds := commands{
		commands: make(map[string]func(*state, command) error),
	}

	cmds.register("login", handlerLogin)

	fmt.Printf("Config after update: %+v\n", cfg)
}
