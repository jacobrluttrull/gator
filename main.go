package main

import (
	"database/sql"
	"log"
	"os"

	_ "github.com/lib/pq"

	"github.com/jacobrluttrull/gator/internal/config"
	"github.com/jacobrluttrull/gator/internal/database"
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

	appState := &state{
		Config: &cfg,
		db:     dbQueries,
	}

	cmds := commands{
		commands: map[string]func(*state, command) error{
			"login":      handlerLogin,
			"register":   registerHandler,
			"reset":      handlerReset,
			"users":      handlerUsers,
			"agg":        handlerAgg,
			"addFeed":    middlewareLoggedIn(handlerAddFeed),
			"feeds":      handlerGetFeeds,
			"follow":     middlewareLoggedIn(handlerFollow),
			"following":  middlewareLoggedIn(handlerFollowing),
			"unfollow":   middlewareLoggedIn(handlerUnfollow),
			"browse":     middlewareLoggedIn(handlerBrowse),
			"bookmark":   middlewareLoggedIn(handlerBookmark),
			"unbookmark": middlewareLoggedIn(handlerUnbookmark),
			"bookmarks":  middlewareLoggedIn(handlerBookmarks),
			"search":     middlewareLoggedIn(handlerSearchPosts),
		},
	}

	if len(os.Args) < 2 {
		log.Fatal("usage: cli <command> [args...]")
	}

	err = cmds.run(appState, command{name: os.Args[1], args: os.Args[2:]})
	if err != nil {
		log.Fatal(err)
	}
}
