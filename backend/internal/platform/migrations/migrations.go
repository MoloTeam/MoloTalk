// Package migrations применяет версионированные миграции схемы к PostgreSQL.
package migrations

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/lock"

	sqlmigrations "github.com/MoloTeam/MoloTalk/backend/migrations"
)

// Stage — этап подготовки, на котором произошёл отказ.
type Stage string

// Этапы подготовки схемы.
const (
	StageDSN     Stage = "dsn"
	StageConnect Stage = "connect"
	StageMigrate Stage = "migrate"
)

// Failure — безопасная запись об отказе. Она не содержит DSN и текст ошибки
// драйвера: они могут включать учётные данные.
type Failure struct {
	Stage    Stage
	Category string
	SQLState string
}

func (f *Failure) Error() string {
	if f.SQLState != "" {
		return fmt.Sprintf("migrations: stage=%s category=%s sqlstate=%s", f.Stage, f.Category, f.SQLState)
	}
	return fmt.Sprintf("migrations: stage=%s category=%s", f.Stage, f.Category)
}

// Open проверяет DSN, открывает пул и подтверждает доступность БД.
func Open(ctx context.Context, dsn string) (*sql.DB, error) {
	if dsn == "" {
		return nil, &Failure{Stage: StageDSN, Category: "database_url_missing"}
	}
	if _, err := pgx.ParseConfig(dsn); err != nil {
		return nil, &Failure{Stage: StageDSN, Category: "database_url_invalid"}
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, &Failure{Stage: StageDSN, Category: "database_url_rejected"}
	}

	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, connectFailure(ctx, err)
	}
	return db, nil
}

// Apply последовательно применяет встроенные миграции к открытому подключению.
func Apply(ctx context.Context, db *sql.DB) error {
	return ApplyFS(ctx, db, sqlmigrations.Files)
}

// ApplyFS применяет миграции из произвольного набора файлов. Проверки
// используют его, чтобы воспроизвести отказ внутри отдельной версии.
func ApplyFS(ctx context.Context, db *sql.DB, fsys fs.FS) error {
	locker, err := lock.NewPostgresSessionLocker()
	if err != nil {
		return &Failure{Stage: StageMigrate, Category: "session_locker_init"}
	}

	provider, err := goose.NewProvider(goose.DialectPostgres, db, fsys, goose.WithSessionLocker(locker))
	if err != nil {
		return &Failure{Stage: StageMigrate, Category: "provider_init"}
	}

	if _, err := provider.Up(ctx); err != nil {
		return migrateFailure(ctx, err)
	}
	return nil
}

// Run открывает подключение, применяет миграции и закрывает его.
func Run(ctx context.Context, dsn string) error {
	db, err := Open(ctx, dsn)
	if err != nil {
		return err
	}
	defer db.Close()

	return Apply(ctx, db)
}

func connectFailure(ctx context.Context, err error) error {
	category := "unreachable"
	if errors.Is(ctx.Err(), context.DeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) {
		category = "unreachable_timeout"
	} else if errors.Is(err, context.Canceled) {
		category = "canceled"
	}
	return &Failure{Stage: StageConnect, Category: category}
}

func migrateFailure(ctx context.Context, err error) error {
	failure := &Failure{Stage: StageMigrate, Category: "sql_error"}

	var pgError *pgconn.PgError
	if errors.As(err, &pgError) {
		failure.SQLState = pgError.Code
	}
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		failure.Category = "timeout"
	}
	return failure
}

// DefaultTimeout — бюджет подготовки схемы по умолчанию.
const DefaultTimeout = 60 * time.Second
