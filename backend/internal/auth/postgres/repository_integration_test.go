//go:build integration

package postgres_test

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/oklog/ulid/v2"

	"github.com/MoloTeam/MoloTalk/backend/internal/auth"
	"github.com/MoloTeam/MoloTalk/backend/internal/auth/postgres"
	"github.com/MoloTeam/MoloTalk/backend/internal/fixtures"
	"github.com/MoloTeam/MoloTalk/backend/internal/platform/clock"
	"github.com/MoloTeam/MoloTalk/backend/internal/platform/idgen"
	"github.com/MoloTeam/MoloTalk/backend/internal/platform/migrations"
	"github.com/MoloTeam/MoloTalk/backend/internal/platform/passwordhash"
	"github.com/MoloTeam/MoloTalk/backend/internal/testenv"
)

func TestMain(m *testing.M) {
	code := m.Run()
	testenv.Terminate()
	os.Exit(code)
}

var registeredAt = time.Date(2026, 11, 26, 12, 0, 0, 0, time.UTC)

func prepare(t *testing.T) *sql.DB {
	t.Helper()

	db := testenv.Shared(t).NewDatabase(t)
	if err := migrations.Apply(context.Background(), db); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
	return db
}

func testID(n byte) string {
	return ulidFromByte(n).String()
}

func newAccount(n byte, identifier string) auth.Account {
	return auth.Account{
		ID:                testID(n),
		Identifier:        identifier,
		DisplayName:       "User",
		PasswordHash:      "$test$hash-" + testID(n),
		CreatedAt:         registeredAt,
		PasswordChangedAt: registeredAt,
	}
}

func count(t *testing.T, db *sql.DB, table string) int {
	t.Helper()

	var total int
	if err := db.QueryRow("SELECT count(*) FROM " + table).Scan(&total); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	return total
}

func TestCreateStoresUserAndCredentials(t *testing.T) {
	db := prepare(t)
	repo := postgres.New(db)
	ctx := context.Background()

	account := newAccount(1, "alice@example.com")
	if err := repo.Create(ctx, account); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := repo.FindByIdentifier(ctx, "alice@example.com")
	if err != nil {
		t.Fatalf("FindByIdentifier: %v", err)
	}
	if got.ID != account.ID || got.Identifier != account.Identifier || got.DisplayName != account.DisplayName {
		t.Errorf("account = %+v, want %+v", got, account)
	}
	if got.PasswordHash != account.PasswordHash {
		t.Errorf("password hash = %q, want %q", got.PasswordHash, account.PasswordHash)
	}
	if !got.CreatedAt.Equal(registeredAt) || !got.PasswordChangedAt.Equal(registeredAt) {
		t.Errorf("times = %v / %v, want %v", got.CreatedAt, got.PasswordChangedAt, registeredAt)
	}
	if count(t, db, "users") != 1 || count(t, db, "user_auth") != 1 {
		t.Errorf("rows: users=%d user_auth=%d, want 1 and 1", count(t, db, "users"), count(t, db, "user_auth"))
	}
}

func TestFindByIdentifierReportsMissingAccount(t *testing.T) {
	repo := postgres.New(prepare(t))

	_, err := repo.FindByIdentifier(context.Background(), "nobody@example.com")
	if !errors.Is(err, auth.ErrAccountNotFound) {
		t.Fatalf("error = %v, want ErrAccountNotFound", err)
	}
}

func TestCreateRejectsDuplicateIdentifier(t *testing.T) {
	db := prepare(t)
	repo := postgres.New(db)
	ctx := context.Background()

	if err := repo.Create(ctx, newAccount(1, "alice@example.com")); err != nil {
		t.Fatalf("first Create: %v", err)
	}
	err := repo.Create(ctx, newAccount(2, "alice@example.com"))
	if !errors.Is(err, auth.ErrIdentifierTaken) {
		t.Fatalf("error = %v, want ErrIdentifierTaken", err)
	}
	if count(t, db, "users") != 1 || count(t, db, "user_auth") != 1 {
		t.Errorf("a rejected duplicate left rows behind: users=%d user_auth=%d",
			count(t, db, "users"), count(t, db, "user_auth"))
	}
}

func TestCreateSurfacesSchemaConstraintsAsStorageErrors(t *testing.T) {
	tests := []struct {
		name    string
		account auth.Account
	}{
		{name: "identifier not lower-case", account: newAccount(1, "Alice@Example.com")},
		{name: "empty identifier", account: newAccount(1, "")},
		{name: "blank display name", account: func() auth.Account {
			account := newAccount(1, "alice@example.com")
			account.DisplayName = "   "
			return account
		}()},
		{name: "malformed id", account: func() auth.Account {
			account := newAccount(1, "alice@example.com")
			account.ID = "not-a-ulid"
			return account
		}()},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			db := prepare(t)
			repo := postgres.New(db)

			err := repo.Create(context.Background(), test.account)
			if err == nil {
				t.Fatal("Create accepted data that violates the schema")
			}
			if errors.Is(err, auth.ErrIdentifierTaken) {
				t.Errorf("a schema violation was reported as a taken identifier: %v", err)
			}
			if count(t, db, "users") != 0 || count(t, db, "user_auth") != 0 {
				t.Errorf("a failed Create left rows behind")
			}
		})
	}
}

func TestConcurrentCreatesOfOneIdentifierSucceedOnce(t *testing.T) {
	db := prepare(t)
	repo := postgres.New(db)
	ctx := context.Background()

	const workers = 8
	results := make(chan error, workers)
	var wg sync.WaitGroup
	for worker := 0; worker < workers; worker++ {
		wg.Add(1)
		go func(n byte) {
			defer wg.Done()
			results <- repo.Create(ctx, newAccount(n, "race@example.com"))
		}(byte(worker + 1))
	}
	wg.Wait()
	close(results)

	created, taken := 0, 0
	for err := range results {
		switch {
		case err == nil:
			created++
		case errors.Is(err, auth.ErrIdentifierTaken):
			taken++
		default:
			t.Errorf("unexpected error: %v", err)
		}
	}
	if created != 1 || taken != workers-1 {
		t.Errorf("created=%d taken=%d, want 1 and %d", created, taken, workers-1)
	}
	if count(t, db, "users") != 1 || count(t, db, "user_auth") != 1 {
		t.Errorf("rows: users=%d user_auth=%d, want 1 and 1", count(t, db, "users"), count(t, db, "user_auth"))
	}
}

func TestFixtureUsersKeepTheirIdentifiersTaken(t *testing.T) {
	db := prepare(t)
	if err := fixtures.Load(context.Background(), db, fixtures.Basic); err != nil {
		t.Fatalf("load fixtures: %v", err)
	}
	repo := postgres.New(db)
	ctx := context.Background()

	found, err := repo.FindByIdentifier(ctx, "alice@example.com")
	if err != nil {
		t.Fatalf("FindByIdentifier: %v", err)
	}
	if found.DisplayName != "Alice" || found.PasswordHash != "" {
		t.Errorf("fixture user = %+v, want Alice without credentials", found)
	}

	err = repo.Create(ctx, newAccount(9, "alice@example.com"))
	if !errors.Is(err, auth.ErrIdentifierTaken) {
		t.Fatalf("error = %v, want ErrIdentifierTaken", err)
	}
}

func TestUpdateReplacesCredentials(t *testing.T) {
	db := prepare(t)
	repo := postgres.New(db)
	ctx := context.Background()

	account := newAccount(1, "alice@example.com")
	if err := repo.Create(ctx, account); err != nil {
		t.Fatalf("Create: %v", err)
	}

	changedAt := registeredAt.Add(48 * time.Hour)
	account.PasswordHash = "$test$new-hash"
	account.PasswordChangedAt = changedAt
	account.DisplayName = "Must not change"
	if err := repo.Update(ctx, account); err != nil {
		t.Fatalf("Update: %v", err)
	}

	got, err := repo.FindByIdentifier(ctx, "alice@example.com")
	if err != nil {
		t.Fatalf("FindByIdentifier: %v", err)
	}
	if got.PasswordHash != "$test$new-hash" || !got.PasswordChangedAt.Equal(changedAt) {
		t.Errorf("credentials = %q at %v, want the updated ones", got.PasswordHash, got.PasswordChangedAt)
	}
	if got.DisplayName != "User" {
		t.Errorf("Update changed the display name to %q", got.DisplayName)
	}

	err = repo.Update(ctx, newAccount(7, "ghost@example.com"))
	if !errors.Is(err, auth.ErrAccountNotFound) {
		t.Errorf("Update of an unknown account: error = %v, want ErrAccountNotFound", err)
	}
}

func TestRegisterOnPostgres(t *testing.T) {
	db := prepare(t)
	hasher := passwordhash.NewArgon2id(passwordhash.Params{
		MemoryKiB:   64,
		Iterations:  1,
		Parallelism: 1,
		SaltLength:  16,
		KeyLength:   32,
	})
	module := auth.New(auth.Deps{
		Accounts: postgres.New(db),
		Hasher:   hasher,
		Clock:    clock.NewManual(registeredAt),
		IDs:      idgen.NewSequence(ulidFromByte(1), ulidFromByte(2)),
	})
	ctx := context.Background()

	account, err := module.Register(ctx, auth.RegisterInput{
		Identifier:  "  Alice@Example.COM ",
		DisplayName: "Alice",
		Password:    "Str0ng!Pass",
	})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if account.Identifier != "alice@example.com" {
		t.Errorf("identifier = %q, want the normalized form", account.Identifier)
	}

	var stored string
	err = db.QueryRow("SELECT password_hash FROM user_auth WHERE user_id = $1::char(26)", account.ID).Scan(&stored)
	if err != nil {
		t.Fatalf("read stored hash: %v", err)
	}
	if stored == "Str0ng!Pass" {
		t.Fatal("the password is stored in plain text")
	}
	if ok, err := hasher.Verify(ctx, "Str0ng!Pass", stored); err != nil || !ok {
		t.Errorf("stored hash does not verify: %v, %v", ok, err)
	}

	_, err = module.Register(ctx, auth.RegisterInput{
		Identifier:  "ALICE@example.com",
		DisplayName: "Impostor",
		Password:    "Str0ng!Pass",
	})
	if !errors.Is(err, auth.ErrIdentifierTaken) {
		t.Errorf("duplicate Register: error = %v, want ErrIdentifierTaken", err)
	}

	_, err = module.Register(ctx, auth.RegisterInput{
		Identifier:  "bob@example.com",
		DisplayName: "Bob",
		Password:    "weak",
	})
	var policy *auth.PasswordPolicyError
	if !errors.As(err, &policy) {
		t.Errorf("weak password: error = %v, want *PasswordPolicyError", err)
	}
	if count(t, db, "users") != 1 || count(t, db, "user_auth") != 1 {
		t.Errorf("rows: users=%d user_auth=%d, want 1 and 1", count(t, db, "users"), count(t, db, "user_auth"))
	}
}

func ulidFromByte(n byte) ulid.ULID {
	var id ulid.ULID
	id[15] = n
	return id
}
