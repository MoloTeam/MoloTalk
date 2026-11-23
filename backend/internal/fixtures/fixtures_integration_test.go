//go:build integration

package fixtures_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"testing"

	"github.com/MoloTeam/MoloTalk/backend/internal/fixtures"
	"github.com/MoloTeam/MoloTalk/backend/internal/platform/migrations"
	"github.com/MoloTeam/MoloTalk/backend/internal/testenv"
)

func TestMain(m *testing.M) {
	code := m.Run()
	testenv.Terminate()
	os.Exit(code)
}

func prepare(t *testing.T) *sql.DB {
	t.Helper()

	db := testenv.Shared(t).NewDatabase(t)
	if err := migrations.Apply(context.Background(), db); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
	return db
}

func loadBasic(t *testing.T, db *sql.DB) {
	t.Helper()

	if err := fixtures.Load(context.Background(), db, fixtures.Basic); err != nil {
		t.Fatalf("load basic: %v", err)
	}
}

func TestBasicIsDeterministicAcrossDatabases(t *testing.T) {
	first := prepare(t)
	second := prepare(t)
	loadBasic(t, first)
	loadBasic(t, second)

	queries := []string{
		`SELECT id, identifier, display_name, created_at FROM users ORDER BY id`,
		`SELECT id, created_by_id, COALESCE(client_request_id::text, ''), COALESCE(create_request_hash, ''),
		        kind, COALESCE(name, ''), COALESCE(owner_id::text, ''), COALESCE(direct_pair_key, ''), created_at
		 FROM channels ORDER BY id`,
		`SELECT channel_id, user_id, role, joined_at FROM channel_members ORDER BY channel_id, user_id`,
	}

	for _, query := range queries {
		firstRows := dump(t, first, query)
		secondRows := dump(t, second, query)
		if firstRows != secondRows {
			t.Fatalf("databases differ for %s\nfirst:\n%s\nsecond:\n%s", query, firstRows, secondRows)
		}
	}
}

func TestBasicRolesAndOwnership(t *testing.T) {
	db := prepare(t)
	loadBasic(t, db)
	ctx := context.Background()

	var owner string
	if err := db.QueryRowContext(ctx, `SELECT owner_id::text FROM channels WHERE id = '01ARZ3NDEKTSV4RRFFQ69G5FA5'`).Scan(&owner); err != nil {
		t.Fatalf("group owner: %v", err)
	}
	if owner != "01ARZ3NDEKTSV4RRFFQ69G5FA1" {
		t.Fatalf("owner = %q, want Alice", owner)
	}

	roles := map[string]string{}
	rows, err := db.QueryContext(ctx, `SELECT user_id::text, role FROM channel_members WHERE channel_id = '01ARZ3NDEKTSV4RRFFQ69G5FA5'`)
	if err != nil {
		t.Fatalf("group roles: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var user, role string
		if err := rows.Scan(&user, &role); err != nil {
			t.Fatalf("scan role: %v", err)
		}
		roles[user] = role
	}
	want := map[string]string{
		"01ARZ3NDEKTSV4RRFFQ69G5FA1": "MEMBER",
		"01ARZ3NDEKTSV4RRFFQ69G5FA2": "ADMIN",
		"01ARZ3NDEKTSV4RRFFQ69G5FA3": "MEMBER",
	}
	for user, role := range want {
		if roles[user] != role {
			t.Fatalf("role of %s = %q, want %q", user, roles[user], role)
		}
	}

	var directMembers int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM channel_members WHERE channel_id = '01ARZ3NDEKTSV4RRFFQ69G5FA4'`).Scan(&directMembers); err != nil {
		t.Fatalf("direct members: %v", err)
	}
	if directMembers != 2 {
		t.Fatalf("direct members = %d, want 2", directMembers)
	}
}

func TestBasicCreatesNoCredentials(t *testing.T) {
	db := prepare(t)
	loadBasic(t, db)
	ctx := context.Background()

	for _, table := range []string{"user_auth", "sessions", "refresh_tokens", "login_attempt_windows", "messages", "attachments", "message_receipts"} {
		var count int
		if err := db.QueryRowContext(ctx, fmt.Sprintf("SELECT count(*) FROM %s", table)).Scan(&count); err != nil {
			t.Fatalf("count %s: %v", table, err)
		}
		if count != 0 {
			t.Fatalf("%s has %d rows, want 0", table, count)
		}
	}

	var users int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM users`).Scan(&users); err != nil {
		t.Fatalf("count users: %v", err)
	}
	if users != 3 {
		t.Fatalf("users = %d, want 3", users)
	}
}

func TestLoaderRefusesNonEmptyTables(t *testing.T) {
	db := prepare(t)
	ctx := context.Background()

	if _, err := db.ExecContext(ctx, `INSERT INTO login_attempt_windows (identifier_hash, failed_count, window_started_at)
		VALUES (repeat('a', 64), 1, '2026-10-19T09:00:00Z')`); err != nil {
		t.Fatalf("insert control row: %v", err)
	}

	err := fixtures.Load(ctx, db, fixtures.Basic)
	if !errors.Is(err, fixtures.ErrNotEmpty) {
		t.Fatalf("error = %v, want ErrNotEmpty", err)
	}

	var windows int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM login_attempt_windows`).Scan(&windows); err != nil {
		t.Fatalf("count windows: %v", err)
	}
	if windows != 1 {
		t.Fatalf("control row was lost: %d", windows)
	}
}

func TestLoaderRollsBackInvalidSet(t *testing.T) {
	db := prepare(t)
	ctx := context.Background()

	statements := []string{
		`INSERT INTO users (id, identifier, display_name, created_at)
		 VALUES ('01ARZ3NDEKTSV4RRFFQ69G5FA1', 'alice@example.com', 'Alice', '2026-10-19T09:00:00Z')`,
		`INSERT INTO user_auth (user_id, password_hash, password_changed_at)
		 VALUES ('01ARZ3NDEKTSV4RRFFQ69G5FA9', 'hash', '2026-10-19T09:00:00Z')`,
	}

	if err := fixtures.LoadStatements(ctx, db, statements); err == nil {
		t.Fatal("invalid set was loaded without an error")
	}

	var users int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM users`).Scan(&users); err != nil {
		t.Fatalf("count users: %v", err)
	}
	if users != 0 {
		t.Fatalf("users = %d after rollback, want 0", users)
	}
}

func TestScenariosAreIsolated(t *testing.T) {
	first := prepare(t)
	second := prepare(t)
	loadBasic(t, first)
	loadBasic(t, second)
	ctx := context.Background()

	if _, err := first.ExecContext(ctx, `UPDATE users SET display_name = 'Robert' WHERE id = '01ARZ3NDEKTSV4RRFFQ69G5FA2'`); err != nil {
		t.Fatalf("update first: %v", err)
	}
	if _, err := first.ExecContext(ctx, `DELETE FROM channel_members WHERE channel_id = '01ARZ3NDEKTSV4RRFFQ69G5FA5' AND user_id = '01ARZ3NDEKTSV4RRFFQ69G5FA2'`); err != nil {
		t.Fatalf("delete membership: %v", err)
	}

	var name string
	if err := second.QueryRowContext(ctx, `SELECT display_name FROM users WHERE id = '01ARZ3NDEKTSV4RRFFQ69G5FA2'`).Scan(&name); err != nil {
		t.Fatalf("read second: %v", err)
	}
	if name != "Bob" {
		t.Fatalf("second database name = %q, want Bob", name)
	}

	var members int
	if err := second.QueryRowContext(ctx, `SELECT count(*) FROM channel_members WHERE channel_id = '01ARZ3NDEKTSV4RRFFQ69G5FA5'`).Scan(&members); err != nil {
		t.Fatalf("count members: %v", err)
	}
	if members != 3 {
		t.Fatalf("second database members = %d, want 3", members)
	}
}

func dump(t *testing.T, db *sql.DB, query string) string {
	t.Helper()

	rows, err := db.QueryContext(context.Background(), query)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		t.Fatalf("columns: %v", err)
	}

	var result string
	for rows.Next() {
		values := make([]any, len(columns))
		pointers := make([]any, len(columns))
		for index := range values {
			pointers[index] = &values[index]
		}
		if err := rows.Scan(pointers...); err != nil {
			t.Fatalf("scan: %v", err)
		}
		result += fmt.Sprint(values...) + "\n"
	}
	return result
}
