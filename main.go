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

	err = cfg.SetUser("jacob")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Config after update: %+v\n", cfg)
}
