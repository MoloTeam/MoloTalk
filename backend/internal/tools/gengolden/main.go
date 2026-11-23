// Команда gengolden строит golden-файл интроспекции из docs/hld/schema.sql.
// Применяются только два принятых отличия: исключение U06 и замена C12.
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/jackc/pgx/v5"
	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/MoloTeam/MoloTalk/backend/internal/platform/migrations"
)

func main() {
	source := flag.String("source", "docs/hld/schema.sql", "path to the schema source")
	dsn := flag.String("dsn", os.Getenv("DATABASE_URL"), "DSN of an empty reference database")
	out := flag.String("out", "backend/internal/platform/migrations/testdata/schema.golden.json", "output path")
	flag.Parse()

	if err := run(context.Background(), *source, *dsn, *out); err != nil {
		fmt.Fprintf(os.Stderr, "gengolden: %v\n", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, source, dsn, out string) error {
	raw, err := os.ReadFile(source)
	if err != nil {
		return err
	}

	target, err := applyTargetEdits(string(raw))
	if err != nil {
		return err
	}

	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		return errors.New("connect failed")
	}
	defer func() { _ = conn.Close(ctx) }()

	if _, err := conn.PgConn().Exec(ctx, target).ReadAll(); err != nil {
		return fmt.Errorf("apply source: %w", err)
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return err
	}
	defer db.Close()

	snapshot, err := migrations.Snapshot(ctx, db)
	if err != nil {
		return err
	}

	encoded, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return err
	}
	encoded = append(encoded, '\n')

	return os.WriteFile(out, encoded, 0o644)
}

// applyTargetEdits воспроизводит два решения владельца поверх источника.
func applyTargetEdits(source string) (string, error) {
	// Источник может содержать CRLF; сравнение идёт по нормализованным строкам.
	source = strings.ReplaceAll(source, "\r\n", "\n")

	const removed = "  UNIQUE (channel_id,id),\n"
	if !strings.Contains(source, removed) {
		return "", errors.New("source has no U06 constraint to remove")
	}
	source = strings.Replace(source, removed, "", 1)

	const originalCheck = "length(text) <= 4000"
	const targetCheck = "char_length(text) <= 4096"
	if !strings.Contains(source, originalCheck) {
		return "", errors.New("source has no C12 check to replace")
	}
	source = strings.Replace(source, originalCheck, targetCheck, 1)

	return source, nil
}
