//go:build integration

// Package testenv поднимает общий PostgreSQL-контейнер для интеграционных
// проверок и выделяет отдельную БД каждому сценарию.
package testenv

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

const (
	image    = "postgres:17-alpine"
	user     = "molotalk"
	password = "molotalk"
	database = "molotalk"
)

// Postgres — общий контейнер набора тестов.
type Postgres struct {
	container *postgres.PostgresContainer
	dsn       string
	sequence  int
	mu        sync.Mutex
}

var (
	once   sync.Once
	shared *Postgres
	err    error
)

// Shared возвращает общий контейнер, запуская его при первом обращении.
func Shared(t *testing.T) *Postgres {
	t.Helper()

	once.Do(func() {
		shared, err = start(context.Background())
	})
	if err != nil {
		t.Fatalf("start postgres container: %v", err)
	}
	return shared
}

// Terminate освобождает контейнер набора.
func Terminate() {
	if shared != nil {
		_ = shared.container.Terminate(context.Background())
	}
}

func start(ctx context.Context) (*Postgres, error) {
	container, err := postgres.Run(ctx, image,
		postgres.WithDatabase(database),
		postgres.WithUsername(user),
		postgres.WithPassword(password),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(90*time.Second),
		),
	)
	if err != nil {
		return nil, err
	}

	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		return nil, err
	}

	return &Postgres{container: container, dsn: dsn}, nil
}

// DSN возвращает строку подключения к БД по умолчанию.
func (p *Postgres) DSN() string {
	return p.dsn
}

// NewDatabase создаёт отдельную БД сценария и регистрирует её удаление.
func (p *Postgres) NewDatabase(t *testing.T) *sql.DB {
	t.Helper()

	scenarioDSN := p.NewDatabaseDSN(t)
	db, err := sql.Open("pgx", scenarioDSN)
	if err != nil {
		t.Fatalf("open scenario connection: %v", err)
	}
	if err := db.Ping(); err != nil {
		t.Fatalf("ping scenario database: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	return db
}

// NewDatabaseDSN создаёт отдельную БД сценария и возвращает её DSN.
// Удаление регистрируется в t.Cleanup.
func (p *Postgres) NewDatabaseDSN(t *testing.T) string {
	t.Helper()

	p.mu.Lock()
	p.sequence++
	name := fmt.Sprintf("scenario_%d_%d", time.Now().UnixNano()%1_000_000, p.sequence)
	p.mu.Unlock()

	admin, err := sql.Open("pgx", p.dsn)
	if err != nil {
		t.Fatalf("open admin connection: %v", err)
	}
	if _, err := admin.Exec("CREATE DATABASE " + name); err != nil {
		admin.Close()
		t.Fatalf("create database: %v", err)
	}

	scenarioDSN, err := databaseDSN(p.dsn, name)
	if err != nil {
		admin.Close()
		t.Fatalf("build scenario dsn: %v", err)
	}

	t.Cleanup(func() {
		_, _ = admin.Exec("DROP DATABASE IF EXISTS " + name + " WITH (FORCE)")
		admin.Close()
	})
	return scenarioDSN
}

func databaseDSN(dsn, name string) (string, error) {
	parsed, err := url.Parse(dsn)
	if err != nil {
		return "", err
	}
	parsed.Path = "/" + name
	return parsed.String(), nil
}

// ExecScript выполняет несколько SQL-команд в одном подключении.
func ExecScript(ctx context.Context, db *sql.DB, script string) error {
	for _, statement := range strings.Split(script, ";") {
		statement = strings.TrimSpace(statement)
		if statement == "" {
			continue
		}
		if _, err := db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("%s: %w", statement, err)
		}
	}
	return nil
}
