// Команда fixtures явно записывает выбранный набор в указанную БД.
package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"os"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/MoloTeam/MoloTalk/backend/internal/fixtures"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("fixtures: loaded")
}

func run() error {
	set := flag.String("set", "", "fixture set to load")
	databaseURL := flag.String("database-url", os.Getenv("DATABASE_URL"), "target database DSN")
	flag.Parse()

	if *set == "" {
		return fmt.Errorf("fixtures: argument -set is required")
	}
	if *databaseURL == "" {
		return fmt.Errorf("fixtures: argument -database-url or DATABASE_URL is required")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	db, err := sql.Open("pgx", *databaseURL)
	if err != nil {
		return fmt.Errorf("fixtures: database_url_invalid")
	}
	defer db.Close()

	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("fixtures: database_unreachable")
	}

	return fixtures.Load(ctx, db, fixtures.Set(*set))
}
