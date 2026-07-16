package main

import (
	"database/sql"
	"log"
	"os"

	_ "github.com/lib/pq"

	"github.com/jacobrluttrull/gator/internal/cli"
	"github.com/jacobrluttrull/gator/internal/config"
	"github.com/jacobrluttrull/gator/internal/database"
	"github.com/jacobrluttrull/gator/internal/handlers"
	"github.com/jacobrluttrull/gator/internal/supervisor"
)

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

	appState := &cli.State{
		Config: &cfg,
		DB:     dbQueries,
	}

	cmds := cli.Commands{
		Handlers: map[string]func(*cli.State, cli.Command) error{
			"login":      handlers.Login,
			"register":   handlers.Register,
			"reset":      handlers.Reset,
			"users":      handlers.Users,
			"agg":        handlers.Agg,
			"addFeed":    cli.LoggedIn(handlers.AddFeed),
			"feeds":      handlers.Feeds,
			"follow":     cli.LoggedIn(handlers.Follow),
			"following":  cli.LoggedIn(handlers.Following),
			"unfollow":   cli.LoggedIn(handlers.Unfollow),
			"browse":     cli.LoggedIn(handlers.Browse),
			"bookmark":   cli.LoggedIn(handlers.Bookmark),
			"unbookmark": cli.LoggedIn(handlers.Unbookmark),
			"bookmarks":  cli.LoggedIn(handlers.Bookmarks),
			"search":     cli.LoggedIn(handlers.Search),
			"serve":      supervisor.Serve,
		},
	}

	if len(os.Args) < 2 {
		log.Fatal("usage: cli <command> [args...]")
	}

	err = cmds.Run(appState, cli.Command{Name: os.Args[1], Args: os.Args[2:]})
	if err != nil {
		log.Fatal(err)
	}
}
