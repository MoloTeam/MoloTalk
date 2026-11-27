//go:build integration

package migrations_test

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	"github.com/MoloTeam/MoloTalk/backend/internal/fixtures"
)

const insertMessage = `INSERT INTO messages (id, channel_id, author_id, client_message_id, text, revision, created_at, request_hash)
	VALUES ($1, $2, $3, $4, $5, $6, '2026-10-19T09:00:00Z', 'hash')`

func withBasic(t *testing.T) *sql.DB {
	t.Helper()

	db := prepare(t)
	if err := fixtures.Load(context.Background(), db, fixtures.Basic); err != nil {
		t.Fatalf("load basic fixture: %v", err)
	}
	return db
}

func TestUniqueConstraintsRejectDuplicates(t *testing.T) {
	db := withBasic(t)
	ctx := context.Background()

	mustExec(t, db, insertMessage, "01ARZ3NDEKTSV4RRFFQ69G5FB1", "01ARZ3NDEKTSV4RRFFQ69G5FA4", "01ARZ3NDEKTSV4RRFFQ69G5FA1", "01ARZ3NDEKTSV4RRFFQ69G5FB2", "hello", 1)

	_, err := db.ExecContext(ctx, `INSERT INTO users (id, identifier, display_name, created_at)
		VALUES ('01ARZ3NDEKTSV4RRFFQ69G5FB3', 'alice@example.com', 'Alice 2', '2026-10-19T09:00:00Z')`)
	expectState(t, err, "23505")

	_, err = db.ExecContext(ctx, `INSERT INTO channels (id, created_by_id, kind, direct_pair_key, created_at)
		VALUES ('01ARZ3NDEKTSV4RRFFQ69G5FB4', '01ARZ3NDEKTSV4RRFFQ69G5FA1', 'DIRECT', 'direct:alice:bob', '2026-10-19T09:00:00Z')`)
	expectState(t, err, "23505")

	_, err = db.ExecContext(ctx, insertMessage, "01ARZ3NDEKTSV4RRFFQ69G5FB5", "01ARZ3NDEKTSV4RRFFQ69G5FA4", "01ARZ3NDEKTSV4RRFFQ69G5FA1", "01ARZ3NDEKTSV4RRFFQ69G5FB2", "hello again", 1)
	expectState(t, err, "23505")
}

func TestForeignKeysRejectMissingRows(t *testing.T) {
	db := withBasic(t)
	ctx := context.Background()

	_, err := db.ExecContext(ctx, insertMessage, "01ARZ3NDEKTSV4RRFFQ69G5FB6", "01ARZ3NDEKTSV4RRFFQ69G5FA4", "01ARZ3NDEKTSV4RRFFQ69G5FB7", "01ARZ3NDEKTSV4RRFFQ69G5FB8", "hello", 1)
	expectState(t, err, "23503")

	_, err = db.ExecContext(ctx, `INSERT INTO channel_members (channel_id, user_id, role, joined_at)
		VALUES ('01ARZ3NDEKTSV4RRFFQ69G5FB9', '01ARZ3NDEKTSV4RRFFQ69G5FA1', 'MEMBER', '2026-10-19T09:00:00Z')`)
	expectState(t, err, "23503")
}

func TestDeferredOwnerRequiresMembership(t *testing.T) {
	db := withBasic(t)
	ctx := context.Background()

	t.Run("owner in the same transaction", func(t *testing.T) {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			t.Fatalf("begin: %v", err)
		}
		defer tx.Rollback()

		if _, err := tx.ExecContext(ctx, `INSERT INTO users (id, identifier, display_name, created_at)
			VALUES ('01ARZ3NDEKTSV4RRFFQ69G5FC1', 'dave@example.com', 'Dave', '2026-10-19T09:00:00Z')`); err != nil {
			t.Fatalf("insert user: %v", err)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO channels (id, created_by_id, client_request_id, create_request_hash, kind, name, owner_id, created_at)
			VALUES ('01ARZ3NDEKTSV4RRFFQ69G5FC2', '01ARZ3NDEKTSV4RRFFQ69G5FC1', '01ARZ3NDEKTSV4RRFFQ69G5FC3', 'hash', 'GROUP', 'Solo', '01ARZ3NDEKTSV4RRFFQ69G5FC1', '2026-10-19T09:00:00Z')`); err != nil {
			t.Fatalf("insert channel: %v", err)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO channel_members (channel_id, user_id, role, joined_at)
			VALUES ('01ARZ3NDEKTSV4RRFFQ69G5FC2', '01ARZ3NDEKTSV4RRFFQ69G5FC1', 'MEMBER', '2026-10-19T09:00:00Z')`); err != nil {
			t.Fatalf("insert member: %v", err)
		}
		if err := tx.Commit(); err != nil {
			t.Fatalf("commit: %v", err)
		}
	})

	t.Run("owner without membership", func(t *testing.T) {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			t.Fatalf("begin: %v", err)
		}
		defer tx.Rollback()

		if _, err := tx.ExecContext(ctx, `INSERT INTO users (id, identifier, display_name, created_at)
			VALUES ('01ARZ3NDEKTSV4RRFFQ69G5FD1', 'erin@example.com', 'Erin', '2026-10-19T09:00:00Z')`); err != nil {
			t.Fatalf("insert user: %v", err)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO channels (id, created_by_id, client_request_id, create_request_hash, kind, name, owner_id, created_at)
			VALUES ('01ARZ3NDEKTSV4RRFFQ69G5FD2', '01ARZ3NDEKTSV4RRFFQ69G5FD1', '01ARZ3NDEKTSV4RRFFQ69G5FD3', 'hash', 'GROUP', 'Orphan', '01ARZ3NDEKTSV4RRFFQ69G5FD1', '2026-10-19T09:00:00Z')`); err != nil {
			t.Fatalf("insert channel: %v", err)
		}
		commitErr := tx.Commit()
		expectState(t, commitErr, "23503")
	})
}

func TestTextLengthBoundaries(t *testing.T) {
	db := withBasic(t)
	ctx := context.Background()

	accepted := strings.Repeat("ж", 4096)
	mustExec(t, db, insertMessage, "01ARZ3NDEKTSV4RRFFQ69G5FE1", "01ARZ3NDEKTSV4RRFFQ69G5FA4", "01ARZ3NDEKTSV4RRFFQ69G5FA1", "01ARZ3NDEKTSV4RRFFQ69G5FE2", accepted, 1)

	rejected := strings.Repeat("ж", 4097)
	_, err := db.ExecContext(ctx, insertMessage, "01ARZ3NDEKTSV4RRFFQ69G5FE3", "01ARZ3NDEKTSV4RRFFQ69G5FA4", "01ARZ3NDEKTSV4RRFFQ69G5FA1", "01ARZ3NDEKTSV4RRFFQ69G5FE4", rejected, 1)
	expectState(t, err, "23514")
}

func TestChecksRejectInvalidValues(t *testing.T) {
	db := withBasic(t)
	ctx := context.Background()

	t.Run("revision", func(t *testing.T) {
		_, err := db.ExecContext(ctx, insertMessage, "01ARZ3NDEKTSV4RRFFQ69G5FF1", "01ARZ3NDEKTSV4RRFFQ69G5FA4", "01ARZ3NDEKTSV4RRFFQ69G5FA1", "01ARZ3NDEKTSV4RRFFQ69G5FF2", "hello", 0)
		expectState(t, err, "23514")
	})

	t.Run("role", func(t *testing.T) {
		_, err := db.ExecContext(ctx, `INSERT INTO channel_members (channel_id, user_id, role, joined_at)
			VALUES ('01ARZ3NDEKTSV4RRFFQ69G5FA5', '01ARZ3NDEKTSV4RRFFQ69G5FA3', 'OWNER', '2026-10-19T09:00:00Z')`)
		expectState(t, err, "23514")
	})

	t.Run("read without delivery", func(t *testing.T) {
		mustExec(t, db, insertMessage, "01ARZ3NDEKTSV4RRFFQ69G5FF3", "01ARZ3NDEKTSV4RRFFQ69G5FA4", "01ARZ3NDEKTSV4RRFFQ69G5FA1", "01ARZ3NDEKTSV4RRFFQ69G5FF4", "hello", 1)

		_, err := db.ExecContext(ctx, `INSERT INTO message_receipts (message_id, user_id, read_at)
			VALUES ('01ARZ3NDEKTSV4RRFFQ69G5FF3', '01ARZ3NDEKTSV4RRFFQ69G5FA2', '2026-10-19T09:01:00Z')`)
		expectState(t, err, "23514")
	})
}

func TestApprovedChecks(t *testing.T) {
	db := withBasic(t)
	ctx := context.Background()

	cases := []struct {
		name  string
		query string
		args  []any
	}{
		{
			name: "C01 ulid",
			query: `INSERT INTO users (id, identifier, display_name, created_at)
				VALUES ('01ARZ3NDEKTSV4RRFFQ69G5F!', 'x@example.com', 'X', '2026-10-19T09:00:00Z')`,
		},
		{
			name: "C02 identifier",
			query: `INSERT INTO users (id, identifier, display_name, created_at)
				VALUES ('01ARZ3NDEKTSV4RRFFQ69G5FG1', 'UPPER@example.com', 'Upper', '2026-10-19T09:00:00Z')`,
		},
		{
			name: "C04 session expiry",
			query: `INSERT INTO sessions (id, user_id, created_at, expires_at)
				VALUES ('01ARZ3NDEKTSV4RRFFQ69G5FG2', '01ARZ3NDEKTSV4RRFFQ69G5FA1', '2026-10-19T09:00:00Z', '2026-10-19T09:00:00Z')`,
		},
		{
			name: "C08 channel name",
			query: `INSERT INTO channels (id, created_by_id, client_request_id, create_request_hash, kind, name, owner_id, created_at)
				VALUES ('01ARZ3NDEKTSV4RRFFQ69G5FG3', '01ARZ3NDEKTSV4RRFFQ69G5FA1', '01ARZ3NDEKTSV4RRFFQ69G5FG4', 'hash', 'GROUP', $1, '01ARZ3NDEKTSV4RRFFQ69G5FA1', '2026-10-19T09:00:00Z')`,
			args: []any{strings.Repeat("n", 101)},
		},
		{
			name: "C15 empty message",
			query: `INSERT INTO messages (id, channel_id, author_id, client_message_id, text, revision, created_at, request_hash)
				VALUES ('01ARZ3NDEKTSV4RRFFQ69G5FG5', '01ARZ3NDEKTSV4RRFFQ69G5FA4', '01ARZ3NDEKTSV4RRFFQ69G5FA1', '01ARZ3NDEKTSV4RRFFQ69G5FG6', '   ', 1, '2026-10-19T09:00:00Z', 'hash')`,
		},
		{
			name: "C17 purpose",
			query: `INSERT INTO attachments (id, uploader_id, purpose, status, object_key, filename, content_type, size_bytes, created_at, upload_expires_at)
				VALUES ('01ARZ3NDEKTSV4RRFFQ69G5FG7', '01ARZ3NDEKTSV4RRFFQ69G5FA1', 'BAD', 'READY', 'k1', 'f', 'text/plain', 1, '2026-10-19T09:00:00Z', '2026-10-19T10:00:00Z')`,
		},
		{
			name: "C18 status",
			query: `INSERT INTO attachments (id, uploader_id, purpose, status, object_key, filename, content_type, size_bytes, created_at, upload_expires_at)
				VALUES ('01ARZ3NDEKTSV4RRFFQ69G5FG8', '01ARZ3NDEKTSV4RRFFQ69G5FA1', 'AVATAR', 'BAD', 'k2', 'f', 'text/plain', 1, '2026-10-19T09:00:00Z', '2026-10-19T10:00:00Z')`,
		},
		{
			name: "C19 size",
			query: `INSERT INTO attachments (id, uploader_id, purpose, status, object_key, filename, content_type, size_bytes, created_at, upload_expires_at)
				VALUES ('01ARZ3NDEKTSV4RRFFQ69G5FG9', '01ARZ3NDEKTSV4RRFFQ69G5FA1', 'AVATAR', 'READY', 'k3', 'f', 'text/plain', -1, '2026-10-19T09:00:00Z', '2026-10-19T10:00:00Z')`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := db.ExecContext(ctx, tc.query, tc.args...)
			expectState(t, err, "23514")
		})
	}
}

func TestApprovedIndexes(t *testing.T) {
	db := prepare(t)
	ctx := context.Background()

	expected := map[string]string{
		"sessions_user_idx":        "user_id",
		"refresh_session_idx":      "session_id",
		"attachment_cleanup_idx":   "status, created_at",
		"login_window_cleanup_idx": "window_started_at",
	}

	for name, columns := range expected {
		var definition string
		if err := db.QueryRowContext(ctx, `SELECT indexdef FROM pg_indexes WHERE schemaname = 'public' AND indexname = $1`, name).Scan(&definition); err != nil {
			t.Fatalf("index %s: %v", name, err)
		}
		if !strings.Contains(definition, columns) {
			t.Fatalf("index %s definition %q lacks %q", name, definition, columns)
		}
	}

	var partial string
	if err := db.QueryRowContext(ctx, `SELECT indexdef FROM pg_indexes WHERE indexname = 'attachment_cleanup_idx'`).Scan(&partial); err != nil {
		t.Fatalf("partial index: %v", err)
	}
	if !strings.Contains(partial, "WHERE (linked_at IS NULL)") {
		t.Fatalf("partial predicate is missing: %s", partial)
	}
}

func TestRejectedU06IsAbsent(t *testing.T) {
	db := prepare(t)
	ctx := context.Background()

	var duplicates int
	if err := db.QueryRowContext(ctx, `
		SELECT count(*)
		FROM pg_constraint
		WHERE conrelid = 'public.messages'::regclass
		  AND contype = 'u'
		  AND pg_get_constraintdef(oid) LIKE '%(channel_id, id)%'`).Scan(&duplicates); err != nil {
		t.Fatalf("check unique constraints: %v", err)
	}
	if duplicates != 0 {
		t.Fatalf("messages(channel_id,id) unique constraint is present")
	}

	var indexes int
	if err := db.QueryRowContext(ctx, `
		SELECT count(*) FROM pg_indexes
		WHERE schemaname = 'public' AND tablename = 'messages' AND indexdef LIKE '%(channel_id, id)%'`).Scan(&indexes); err != nil {
		t.Fatalf("check indexes: %v", err)
	}
	if indexes != 0 {
		t.Fatalf("messages(channel_id,id) index is present")
	}

	var referencing int
	if err := db.QueryRowContext(ctx, `
		SELECT count(*)
		FROM pg_constraint
		WHERE contype = 'f' AND confrelid = 'public.messages'::regclass`).Scan(&referencing); err != nil {
		t.Fatalf("check foreign keys: %v", err)
	}
	if referencing != 1 {
		t.Fatalf("foreign keys referencing messages = %d, want 1", referencing)
	}

	var width int
	if err := db.QueryRowContext(ctx, `
		SELECT cardinality(confkey)
		FROM pg_constraint
		WHERE contype = 'f' AND confrelid = 'public.messages'::regclass`).Scan(&width); err != nil {
		t.Fatalf("check key width: %v", err)
	}
	if width != 1 {
		t.Fatalf("foreign key references %d columns, want 1", width)
	}
}
