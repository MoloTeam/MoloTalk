//go:build integration

package migrations_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/MoloTeam/MoloTalk/backend/internal/platform/migrations"
	"github.com/MoloTeam/MoloTalk/backend/internal/testenv"
)

func TestMain(m *testing.M) {
	code := m.Run()
	testenv.Terminate()
	os.Exit(code)
}

// prepare возвращает отдельную БД с применёнными миграциями.
func prepare(t *testing.T) *sql.DB {
	t.Helper()

	db := testenv.Shared(t).NewDatabase(t)
	if err := migrations.Apply(context.Background(), db); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
	return db
}

func TestSchemaMatchesGolden(t *testing.T) {
	db := prepare(t)

	snapshot, err := migrations.Snapshot(context.Background(), db)
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}

	raw, err := os.ReadFile(filepath.Join("testdata", "schema.golden.json"))
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}
	var golden migrations.Schema
	if err := json.Unmarshal(raw, &golden); err != nil {
		t.Fatalf("decode golden: %v", err)
	}

	if !reflect.DeepEqual(snapshot, golden) {
		compare, _ := json.MarshalIndent(snapshot, "", "  ")
		t.Fatalf("schema does not match golden\nactual:\n%s", compare)
	}
}

func TestReapplyKeepsData(t *testing.T) {
	db := prepare(t)
	ctx := context.Background()

	mustExec(t, db, `INSERT INTO users (id, identifier, display_name, created_at)
		VALUES ('01ARZ3NDEKTSV4RRFFQ69G5FA1', 'alice@example.com', 'Alice', '2026-10-19T09:00:00Z')`)

	if err := migrations.Apply(ctx, db); err != nil {
		t.Fatalf("second apply: %v", err)
	}

	var versions int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM goose_db_version WHERE version_id = 1`).Scan(&versions); err != nil {
		t.Fatalf("count versions: %v", err)
	}
	if versions != 1 {
		t.Fatalf("version 1 recorded %d times, want 1", versions)
	}

	var displayName string
	if err := db.QueryRowContext(ctx, `SELECT display_name FROM users WHERE id = '01ARZ3NDEKTSV4RRFFQ69G5FA1'`).Scan(&displayName); err != nil {
		t.Fatalf("control row: %v", err)
	}
	if displayName != "Alice" {
		t.Fatalf("display_name = %q, want Alice", displayName)
	}
}

func TestConcurrentApplySerializes(t *testing.T) {
	db := testenv.Shared(t).NewDatabase(t)
	ctx := context.Background()

	var wg sync.WaitGroup
	errs := make([]error, 2)
	for index := range errs {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			errs[index] = migrations.Apply(ctx, db)
		}(index)
	}
	wg.Wait()

	for index, err := range errs {
		if err != nil {
			t.Fatalf("migrator %d: %v", index, err)
		}
	}

	var versions int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM goose_db_version WHERE version_id = 1`).Scan(&versions); err != nil {
		t.Fatalf("count versions: %v", err)
	}
	if versions != 1 {
		t.Fatalf("version 1 recorded %d times, want 1", versions)
	}
}

func TestBrokenMigrationRollsBack(t *testing.T) {
	db := prepare(t)
	ctx := context.Background()

	broken := fstest.MapFS{
		"00002_broken.sql": &fstest.MapFile{Data: []byte(`-- +goose Up
CREATE TABLE broken_table (id integer);
INSERT INTO missing_table VALUES (1);

-- +goose Down
DROP TABLE broken_table;
`)},
	}

	err := migrations.ApplyFS(ctx, db, broken)
	if err == nil {
		t.Fatal("broken migration was applied without an error")
	}

	var exists sql.NullString
	if err := db.QueryRowContext(ctx, `SELECT to_regclass('public.broken_table')::text`).Scan(&exists); err != nil {
		t.Fatalf("check broken table: %v", err)
	}
	if exists.Valid {
		t.Fatalf("broken migration left table %q", exists.String)
	}

	var recorded int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM goose_db_version WHERE version_id = 2`).Scan(&recorded); err != nil {
		t.Fatalf("count version 2: %v", err)
	}
	if recorded != 0 {
		t.Fatalf("version 2 recorded %d times, want 0", recorded)
	}

	var versions int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM goose_db_version WHERE version_id = 1`).Scan(&versions); err != nil {
		t.Fatalf("count version 1: %v", err)
	}
	if versions != 1 {
		t.Fatalf("version 1 lost after failure: %d", versions)
	}
}

func TestMigrateDiagnosticsHidesSecret(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "migrate.exe")
	build := exec.Command("go", "build", "-o", binary, "../../../cmd/migrate")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build migrate: %v\n%s", err, output)
	}

	const secret = "supersecret-marker-9f3c"

	cases := []struct {
		name string
		dsn  string
	}{
		{name: "invalid dsn", dsn: secret},
		{name: "unreachable", dsn: "postgres://molotalk:" + secret + "@127.0.0.1:1/molotalk?sslmode=disable&connect_timeout=1"},
	}

	for _, tc := range cases {
		command := exec.Command(binary)
		command.Env = append(os.Environ(), "DATABASE_URL="+tc.dsn, "MIGRATE_TIMEOUT=3s")

		started := time.Now()
		output, err := command.CombinedOutput()
		elapsed := time.Since(started)

		if err == nil {
			t.Fatalf("%s: command succeeded, want failure", tc.name)
		}
		if elapsed > 30*time.Second {
			t.Fatalf("%s: command took %v", tc.name, elapsed)
		}
		if strings.Contains(string(output), secret) {
			t.Fatalf("%s: output leaks the secret:\n%s", tc.name, output)
		}
		if strings.Contains(string(output), "postgres://") {
			t.Fatalf("%s: output leaks the DSN:\n%s", tc.name, output)
		}
	}
}

func mustExec(t *testing.T, db *sql.DB, query string, args ...any) {
	t.Helper()

	if _, err := db.ExecContext(context.Background(), query, args...); err != nil {
		t.Fatalf("exec: %v\n%s", err, query)
	}
}

func expectState(t *testing.T, err error, want string) {
	t.Helper()

	if err == nil {
		t.Fatalf("expected SQLSTATE %s, got no error", want)
	}
	var pgError *pgconn.PgError
	if !errors.As(err, &pgError) {
		t.Fatalf("expected SQLSTATE %s, got %v", want, err)
	}
	if pgError.Code != want {
		t.Fatalf("SQLSTATE = %s, want %s (%v)", pgError.Code, want, err)
	}
}
