// Команда migrate применяет версии схемы к БД из DATABASE_URL.
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/MoloTeam/MoloTalk/backend/internal/platform/migrations"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("migrations: applied")
}

func run() error {
	timeout := migrations.DefaultTimeout
	if raw := os.Getenv("MIGRATE_TIMEOUT"); raw != "" {
		parsed, err := time.ParseDuration(raw)
		if err != nil {
			return fmt.Errorf("migrations: stage=config category=timeout_invalid")
		}
		timeout = parsed
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	return migrations.Run(ctx, os.Getenv("DATABASE_URL"))
}
