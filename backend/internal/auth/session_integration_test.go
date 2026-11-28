//go:build integration

package auth_test

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/oklog/ulid/v2"

	"github.com/MoloTeam/MoloTalk/backend/internal/auth"
	"github.com/MoloTeam/MoloTalk/backend/internal/auth/access"
	"github.com/MoloTeam/MoloTalk/backend/internal/auth/password"
	authpostgres "github.com/MoloTeam/MoloTalk/backend/internal/auth/postgres"
	"github.com/MoloTeam/MoloTalk/backend/internal/auth/refresh"
	"github.com/MoloTeam/MoloTalk/backend/internal/platform/clock"
	"github.com/MoloTeam/MoloTalk/backend/internal/platform/idgen"
	"github.com/MoloTeam/MoloTalk/backend/internal/platform/migrations"
	"github.com/MoloTeam/MoloTalk/backend/internal/testenv"
)

const testPassword = "Secret1!"

var testHasherParams = password.Params{
	Memory:      8 * 1024,
	Iterations:  1,
	Parallelism: 1,
	SaltLength:  16,
	KeyLength:   32,
}

type fixture struct {
	module *auth.Module
	store  *authpostgres.Store
	pool   *pgxpool.Pool
	clock  *clock.Manual
	codec  *access.Codec
}

func newFixture(t *testing.T) *fixture {
	t.Helper()

	ctx := context.Background()
	dsn := testenv.Shared(t).NewDatabaseDSN(t)
	if err := migrations.Run(ctx, dsn); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("open pool: %v", err)
	}
	t.Cleanup(pool.Close)

	_, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate signing key: %v", err)
	}

	manual := clock.NewManual(fixedTime)
	store := authpostgres.New(pool)
	codec := access.NewCodec(private, manual)
	module := auth.New(auth.Deps{
		Accounts:      store,
		Sessions:      store,
		Hasher:        password.New(testHasherParams),
		Tokens:        codec,
		RefreshTokens: refresh.New(),
		Clock:         manual,
		IDs:           idgen.NewSystem(manual),
	})

	return &fixture{module: module, store: store, pool: pool, clock: manual, codec: codec}
}

func (f *fixture) register(t *testing.T, identifier string) auth.TokenPair {
	t.Helper()

	pair, err := f.module.Register(context.Background(), auth.RegisterInput{
		Identifier:  identifier,
		DisplayName: "Alice",
		Password:    testPassword,
	})
	if err != nil {
		t.Fatalf("register %q: %v", identifier, err)
	}
	return pair
}

func (f *fixture) count(t *testing.T, query string, args ...any) int {
	t.Helper()

	var count int
	if err := f.pool.QueryRow(context.Background(), query, args...).Scan(&count); err != nil {
		t.Fatalf("query count: %v", err)
	}
	return count
}

func (f *fixture) addSession(t *testing.T, accountID, sessionID, refreshToken string) {
	t.Helper()

	ctx := context.Background()
	expires := f.clock.Now().UTC().Add(auth.SessionTTL)
	if _, err := f.pool.Exec(ctx,
		`INSERT INTO sessions (id, user_id, created_at, expires_at) VALUES ($1, $2, $3, $4)`,
		sessionID, accountID, f.clock.Now().UTC(), expires,
	); err != nil {
		t.Fatalf("insert session: %v", err)
	}
	if _, err := f.pool.Exec(ctx,
		`INSERT INTO refresh_tokens (token_hash, session_id, expires_at) VALUES ($1, $2, $3)`,
		auth.HashRefreshToken(refreshToken), sessionID, expires,
	); err != nil {
		t.Fatalf("insert refresh token: %v", err)
	}
}

func TestMain(m *testing.M) {
	code := m.Run()
	testenv.Terminate()
	os.Exit(code)
}

func TestRegisterStoresMatchingRecords(t *testing.T) {
	f := newFixture(t)
	pair := f.register(t, " Alice@Example.COM ")

	if pair.Profile.Identifier != "alice@example.com" {
		t.Errorf("identifier = %q, want canonical lower case", pair.Profile.Identifier)
	}
	if pair.RefreshExpiresAt != f.clock.Now().UTC().Add(auth.SessionTTL) {
		t.Errorf("refresh expiry = %v, want registration time + 30 days", pair.RefreshExpiresAt)
	}

	claims, err := f.codec.Validate(context.Background(), pair.AccessToken)
	if err != nil {
		t.Fatalf("validate access token: %v", err)
	}
	if claims.AccountID != pair.Profile.ID || claims.SessionID != pair.SessionID {
		t.Errorf("access claims = %+v, pair = %+v", claims, pair)
	}

	var passwordHash string
	if err := f.pool.QueryRow(context.Background(),
		`SELECT password_hash FROM user_auth WHERE user_id = $1`, pair.Profile.ID,
	).Scan(&passwordHash); err != nil {
		t.Fatalf("read password hash: %v", err)
	}
	if passwordHash == testPassword {
		t.Fatal("password stored in plain text")
	}
	hasher := password.New(testHasherParams)
	if ok, err := hasher.Verify(context.Background(), testPassword, passwordHash); err != nil || !ok {
		t.Fatalf("stored password hash does not verify: %v, %v", ok, err)
	}

	var sessionExpires time.Time
	if err := f.pool.QueryRow(context.Background(),
		`SELECT expires_at FROM sessions WHERE id = $1 AND user_id = $2`,
		pair.SessionID, pair.Profile.ID,
	).Scan(&sessionExpires); err != nil {
		t.Fatalf("read session: %v", err)
	}
	if !sessionExpires.Equal(pair.RefreshExpiresAt) {
		t.Errorf("session expiry = %v, want %v", sessionExpires, pair.RefreshExpiresAt)
	}

	var refreshExpires time.Time
	if err := f.pool.QueryRow(context.Background(),
		`SELECT expires_at FROM refresh_tokens WHERE token_hash = $1 AND session_id = $2`,
		auth.HashRefreshToken(pair.RefreshToken), pair.SessionID,
	).Scan(&refreshExpires); err != nil {
		t.Fatalf("read refresh token: %v", err)
	}
	if !refreshExpires.Equal(pair.RefreshExpiresAt) {
		t.Errorf("refresh expiry = %v, want %v", refreshExpires, pair.RefreshExpiresAt)
	}

	if count := f.count(t, `SELECT count(*) FROM refresh_tokens WHERE token_hash = $1`, pair.RefreshToken); count != 0 {
		t.Error("raw refresh token is stored in the database")
	}
}

func TestRegisterRejectsConcurrentIdentifier(t *testing.T) {
	f := newFixture(t)

	errs := make([]error, 2)
	var wait sync.WaitGroup
	for index := range errs {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			_, errs[index] = f.module.Register(context.Background(), auth.RegisterInput{
				Identifier:  "race@example.com",
				DisplayName: "Race",
				Password:    testPassword,
			})
		}(index)
	}
	wait.Wait()

	var succeeded, conflicted int
	for _, err := range errs {
		switch {
		case err == nil:
			succeeded++
		case errors.Is(err, auth.ErrIdentifierTaken):
			conflicted++
		default:
			t.Errorf("unexpected registration error: %v", err)
		}
	}
	if succeeded != 1 || conflicted != 1 {
		t.Fatalf("results = %d success / %d conflict, want 1/1", succeeded, conflicted)
	}
	if count := f.count(t, `SELECT count(*) FROM users WHERE identifier = 'race@example.com'`); count != 1 {
		t.Errorf("users = %d, want 1", count)
	}
	if count := f.count(t, `SELECT count(*) FROM sessions`); count != 1 {
		t.Errorf("sessions = %d, want 1", count)
	}
}

func TestRegisterRollsBackAfterSessionFailure(t *testing.T) {
	f := newFixture(t)
	existing := f.register(t, "alice@example.com")

	newAccount, err := ulid.New(ulid.Timestamp(f.clock.Now()), rand.Reader)
	if err != nil {
		t.Fatalf("new ULID: %v", err)
	}
	collidingModule := auth.New(auth.Deps{
		Accounts:      f.store,
		Sessions:      f.store,
		Hasher:        password.New(testHasherParams),
		Tokens:        f.codec,
		RefreshTokens: refresh.New(),
		Clock:         f.clock,
		IDs: idgen.NewSequence(
			newAccount,
			ulid.MustParse(existing.SessionID),
		),
	})

	_, err = collidingModule.Register(context.Background(), auth.RegisterInput{
		Identifier:  "bob@example.com",
		DisplayName: "Bob",
		Password:    testPassword,
	})
	if err == nil {
		t.Fatal("registration succeeded despite the session id collision")
	}

	if count := f.count(t, `SELECT count(*) FROM users WHERE identifier = 'bob@example.com'`); count != 0 {
		t.Errorf("users for rolled-back registration = %d, want 0", count)
	}
	if count := f.count(t, `SELECT count(*) FROM refresh_tokens WHERE session_id = $1`, existing.SessionID); count != 1 {
		t.Errorf("refresh tokens for the existing session = %d, want 1", count)
	}
}

func TestRefreshRotatesSameSessionAndExtendsIt(t *testing.T) {
	f := newFixture(t)
	initial := f.register(t, "alice@example.com")
	initialExpiry := f.clock.Now().UTC().Add(auth.SessionTTL)

	f.clock.Advance(24 * time.Hour)
	rotated, err := f.module.Refresh(context.Background(), initial.RefreshToken)
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}

	if rotated.SessionID != initial.SessionID || rotated.Profile.ID != initial.Profile.ID {
		t.Errorf("rotation changed the session: %+v", rotated)
	}
	if want := f.clock.Now().UTC().Add(auth.SessionTTL); !rotated.RefreshExpiresAt.Equal(want) {
		t.Errorf("refresh expiry = %v, want %v", rotated.RefreshExpiresAt, want)
	}
	if !rotated.RefreshExpiresAt.After(initialExpiry) {
		t.Errorf("session was not extended: %v <= %v", rotated.RefreshExpiresAt, initialExpiry)
	}

	var consumed pgtype.Timestamptz
	if err := f.pool.QueryRow(context.Background(),
		`SELECT consumed_at FROM refresh_tokens WHERE token_hash = $1`,
		auth.HashRefreshToken(initial.RefreshToken),
	).Scan(&consumed); err != nil {
		t.Fatalf("read consumed token: %v", err)
	}
	if !consumed.Valid {
		t.Fatal("old refresh token was not marked consumed")
	}
	if count := f.count(t, `SELECT count(*) FROM refresh_tokens WHERE token_hash = $1`,
		auth.HashRefreshToken(rotated.RefreshToken)); count != 1 {
		t.Errorf("new refresh hash rows = %d, want 1", count)
	}
	if count := f.count(t, `SELECT count(*) FROM refresh_tokens WHERE token_hash = $1`,
		rotated.RefreshToken); count != 0 {
		t.Error("raw rotated token is stored in the database")
	}
}

func TestRefreshRollsBackConsumptionOnInsertFailure(t *testing.T) {
	f := newFixture(t)
	pair := f.register(t, "alice@example.com")
	hash := auth.HashRefreshToken(pair.RefreshToken)

	var expiresBefore time.Time
	if err := f.pool.QueryRow(context.Background(),
		`SELECT expires_at FROM sessions WHERE id = $1`, pair.SessionID,
	).Scan(&expiresBefore); err != nil {
		t.Fatalf("read session expiry: %v", err)
	}

	// Хеш нового токена совпадает с существующим: INSERT нарушает PRIMARY KEY
	// после отметки о погашении, поэтому транзакция должна откатиться.
	_, err := f.store.RotateRefresh(context.Background(), auth.Rotation{
		TokenHash:        hash,
		NewTokenHash:     hash,
		Now:              f.clock.Now().UTC(),
		SessionExpiresAt: f.clock.Now().UTC().Add(auth.SessionTTL),
	}, f.codec.Issue)
	if err == nil {
		t.Fatal("rotation succeeded despite the hash collision")
	}

	var consumed pgtype.Timestamptz
	if err := f.pool.QueryRow(context.Background(),
		`SELECT consumed_at FROM refresh_tokens WHERE token_hash = $1`, hash,
	).Scan(&consumed); err != nil {
		t.Fatalf("read token: %v", err)
	}
	if consumed.Valid {
		t.Error("failed rotation left the old token consumed")
	}
	var expiresAfter time.Time
	if err := f.pool.QueryRow(context.Background(),
		`SELECT expires_at FROM sessions WHERE id = $1`, pair.SessionID,
	).Scan(&expiresAfter); err != nil {
		t.Fatalf("read session expiry: %v", err)
	}
	if !expiresAfter.Equal(expiresBefore) {
		t.Errorf("failed rotation changed session expiry: %v -> %v", expiresBefore, expiresAfter)
	}
}

type failingSigner struct {
	auth.TokenCodec
	err error
}

func (s failingSigner) Issue(context.Context, auth.Claims) (auth.AccessToken, error) {
	return auth.AccessToken{}, s.err
}

func TestRefreshSigningFailureLeavesOriginalSessionUsable(t *testing.T) {
	f := newFixture(t)
	pair := f.register(t, "alice@example.com")
	f.clock.Advance(time.Hour)
	failure := errors.New("test signing failure")
	module := auth.New(auth.Deps{
		Sessions:      f.store,
		Tokens:        failingSigner{TokenCodec: f.codec, err: failure},
		RefreshTokens: refresh.New(),
		Clock:         f.clock,
	})
	brokenPair, err := module.Refresh(t.Context(), pair.RefreshToken)
	if !errors.Is(err, failure) || brokenPair.AccessToken != "" || brokenPair.RefreshToken != "" {
		t.Fatalf("failed signing returned an unexpected result: %v", err)
	}
	if count := f.count(t, `SELECT count(*) FROM refresh_tokens WHERE session_id = $1`, pair.SessionID); count != 1 {
		t.Fatalf("failed signing left %d refresh records, want 1", count)
	}
	var expiry time.Time
	if err := f.pool.QueryRow(t.Context(), `SELECT expires_at FROM sessions WHERE id = $1`, pair.SessionID).Scan(&expiry); err != nil {
		t.Fatalf("read session expiry: %v", err)
	}
	if !expiry.Equal(pair.RefreshExpiresAt) {
		t.Fatalf("failed signing extended the session to %v, want %v", expiry, pair.RefreshExpiresAt)
	}
	if next, err := f.module.Refresh(t.Context(), pair.RefreshToken); err != nil || next.SessionID != pair.SessionID {
		t.Fatalf("original refresh no longer works: %v", err)
	}
}

func TestRefreshRejectsUnknownExpiredAndInactive(t *testing.T) {
	f := newFixture(t)

	if _, err := f.module.Refresh(context.Background(), "unknown-token"); !errors.Is(err, auth.ErrUnauthenticated) {
		t.Errorf("unknown token error = %v, want unauthenticated", err)
	}

	expired := f.register(t, "expired@example.com")
	f.clock.Advance(auth.SessionTTL + time.Second)
	if _, err := f.module.Refresh(context.Background(), expired.RefreshToken); !errors.Is(err, auth.ErrUnauthenticated) {
		t.Errorf("expired token error = %v, want unauthenticated", err)
	}

	inactive := f.register(t, "inactive@example.com")
	if err := f.module.Logout(context.Background(), auth.Claims{
		AccountID: inactive.Profile.ID,
		SessionID: inactive.SessionID,
	}, false); err != nil {
		t.Fatalf("Logout: %v", err)
	}
	if _, err := f.module.Refresh(context.Background(), inactive.RefreshToken); !errors.Is(err, auth.ErrUnauthenticated) {
		t.Errorf("inactive session error = %v, want unauthenticated", err)
	}
}

func TestReplayRevokesSessionBeforeResponse(t *testing.T) {
	f := newFixture(t)
	initial := f.register(t, "alice@example.com")
	rotated, err := f.module.Refresh(context.Background(), initial.RefreshToken)
	if err != nil {
		t.Fatalf("first Refresh: %v", err)
	}

	if _, err := f.module.Refresh(context.Background(), initial.RefreshToken); !errors.Is(err, auth.ErrUnauthenticated) {
		t.Errorf("replay error = %v, want unauthenticated", err)
	}

	var revoked pgtype.Timestamptz
	if err := f.pool.QueryRow(context.Background(),
		`SELECT revoked_at FROM sessions WHERE id = $1`, initial.SessionID,
	).Scan(&revoked); err != nil {
		t.Fatalf("read session: %v", err)
	}
	if !revoked.Valid {
		t.Fatal("replay did not commit the session revocation")
	}
	if _, err := f.module.Refresh(context.Background(), rotated.RefreshToken); !errors.Is(err, auth.ErrUnauthenticated) {
		t.Errorf("rotated token after replay error = %v, want unauthenticated", err)
	}
}

func TestExpiredConsumedTokenStillRevokesActiveSession(t *testing.T) {
	f := newFixture(t)
	initial := f.register(t, "alice@example.com")

	f.clock.Advance(time.Hour)
	if _, err := f.module.Refresh(context.Background(), initial.RefreshToken); err != nil {
		t.Fatalf("Refresh: %v", err)
	}

	// Исходный токен истёк, но продлённый сеанс ещё действует.
	f.clock.Advance(auth.SessionTTL - 30*time.Minute)
	if _, err := f.module.Refresh(context.Background(), initial.RefreshToken); !errors.Is(err, auth.ErrUnauthenticated) {
		t.Fatalf("expired replay error = %v, want unauthenticated", err)
	}

	var revoked pgtype.Timestamptz
	if err := f.pool.QueryRow(context.Background(),
		`SELECT revoked_at FROM sessions WHERE id = $1`, initial.SessionID,
	).Scan(&revoked); err != nil {
		t.Fatalf("read session: %v", err)
	}
	if !revoked.Valid {
		t.Fatal("expired consumed token did not revoke the active session")
	}
}

func TestConcurrentRefreshProducesOnePairAndRevokesSession(t *testing.T) {
	f := newFixture(t)
	initial := f.register(t, "alice@example.com")

	start := make(chan struct{})
	results := make([]error, 2)
	var wait sync.WaitGroup
	for index := range results {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			<-start
			_, results[index] = f.module.Refresh(context.Background(), initial.RefreshToken)
		}(index)
	}
	close(start)
	wait.Wait()

	success, rejected := 0, 0
	for _, err := range results {
		switch {
		case err == nil:
			success++
		case errors.Is(err, auth.ErrUnauthenticated):
			rejected++
		default:
			t.Errorf("unexpected refresh error: %v", err)
		}
	}
	if success != 1 || rejected != 1 {
		t.Fatalf("results = %d success / %d rejected, want 1/1", success, rejected)
	}

	var revoked pgtype.Timestamptz
	if err := f.pool.QueryRow(context.Background(),
		`SELECT revoked_at FROM sessions WHERE id = $1`, initial.SessionID,
	).Scan(&revoked); err != nil {
		t.Fatalf("read session: %v", err)
	}
	if !revoked.Valid {
		t.Fatal("concurrent replay did not revoke the session")
	}
}

func TestLogoutCurrentAllAndAccessExpiry(t *testing.T) {
	f := newFixture(t)
	first := f.register(t, "alice@example.com")
	secondSessionID := "01ARZ3NDEKTSV4RRFFQ69G5FB1"
	secondToken := "second-session-refresh"
	f.addSession(t, first.Profile.ID, secondSessionID, secondToken)

	if err := f.module.Logout(context.Background(), auth.Claims{
		AccountID: first.Profile.ID,
		SessionID: first.SessionID,
	}, false); err != nil {
		t.Fatalf("Logout current: %v", err)
	}
	if _, err := f.module.Refresh(context.Background(), first.RefreshToken); !errors.Is(err, auth.ErrUnauthenticated) {
		t.Errorf("refresh after current logout = %v, want unauthenticated", err)
	}
	if _, err := f.module.Refresh(context.Background(), secondToken); err != nil {
		t.Errorf("second session refresh after current logout: %v", err)
	}
	if err := f.module.Logout(context.Background(), auth.Claims{
		AccountID: first.Profile.ID,
		SessionID: first.SessionID,
	}, false); err != nil {
		t.Errorf("repeated logout: %v", err)
	}

	other := f.register(t, "bob@example.com")
	if err := f.module.Logout(context.Background(), auth.Claims{
		AccountID: first.Profile.ID,
		SessionID: first.SessionID,
	}, true); err != nil {
		t.Fatalf("Logout all: %v", err)
	}
	if _, err := f.module.Refresh(context.Background(), secondToken); !errors.Is(err, auth.ErrUnauthenticated) {
		t.Errorf("refresh after logout all = %v, want unauthenticated", err)
	}
	otherRotated, err := f.module.Refresh(context.Background(), other.RefreshToken)
	if err != nil {
		t.Fatalf("other user's session was revoked: %v", err)
	}

	// Ранее выданный access проверяется локально и действует до exp, даже
	// если сеанс уже отозван.
	if _, err := f.codec.Validate(context.Background(), first.AccessToken); err != nil {
		t.Errorf("access before exp after logout: %v", err)
	}
	f.clock.Advance(auth.AccessTokenTTL)
	if _, err := f.codec.Validate(context.Background(), first.AccessToken); err == nil {
		t.Error("access still valid at exp")
	}
	if otherRotated.SessionID == "" {
		t.Error("other user's rotation did not return a session")
	}
}

func TestLogoutAllRacesWithRotation(t *testing.T) {
	f := newFixture(t)
	pair := f.register(t, "alice@example.com")

	start := make(chan struct{})
	var wait sync.WaitGroup
	var refreshErr, logoutErr error
	wait.Add(2)
	go func() {
		defer wait.Done()
		<-start
		_, refreshErr = f.module.Refresh(context.Background(), pair.RefreshToken)
	}()
	go func() {
		defer wait.Done()
		<-start
		logoutErr = f.module.Logout(context.Background(), auth.Claims{
			AccountID: pair.Profile.ID,
			SessionID: pair.SessionID,
		}, true)
	}()
	close(start)
	wait.Wait()

	if logoutErr != nil {
		t.Fatalf("Logout all: %v", logoutErr)
	}
	if refreshErr != nil && !errors.Is(refreshErr, auth.ErrUnauthenticated) {
		t.Fatalf("refresh error = %v", refreshErr)
	}
	// После согласованного завершения refresh сеанса невозможен.
	if _, err := f.module.Refresh(context.Background(), pair.RefreshToken); !errors.Is(err, auth.ErrUnauthenticated) {
		t.Errorf("refresh after concurrent logout = %v, want unauthenticated", err)
	}
}
