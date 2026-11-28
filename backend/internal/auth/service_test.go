package auth_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/oklog/ulid/v2"

	"github.com/MoloTeam/MoloTalk/backend/internal/auth"
	"github.com/MoloTeam/MoloTalk/backend/internal/platform/clock"
	"github.com/MoloTeam/MoloTalk/backend/internal/platform/idgen"
)

var fixedTime = time.Date(2026, time.November, 28, 12, 0, 0, 0, time.UTC)

func mustULID(t *testing.T, value string) ulid.ULID {
	t.Helper()
	parsed, err := ulid.Parse(value)
	if err != nil {
		t.Fatalf("parse ULID %q: %v", value, err)
	}
	return parsed
}

func dependencies(t *testing.T) (*accounts, *sessions, *hasher, *tokens, *refreshTokens, auth.Deps) {
	t.Helper()

	accountStore := &accounts{}
	sessionStore := &sessions{}
	passwordHasher := &hasher{hash: "encoded-hash"}
	tokenCodec := &tokens{expires: fixedTime.Add(auth.AccessTokenTTL)}
	refreshGenerator := &refreshTokens{token: "refresh-token", hash: auth.HashRefreshToken("refresh-token")}

	return accountStore, sessionStore, passwordHasher, tokenCodec, refreshGenerator, auth.Deps{
		Accounts:      accountStore,
		Sessions:      sessionStore,
		Hasher:        passwordHasher,
		Tokens:        tokenCodec,
		RefreshTokens: refreshGenerator,
		Clock:         clock.NewManual(fixedTime),
		IDs: idgen.NewSequence(
			mustULID(t, "01ARZ3NDEKTSV4RRFFQ69G5FA1"),
			mustULID(t, "01ARZ3NDEKTSV4RRFFQ69G5FA2"),
		),
	}
}

func TestRegisterPersistsNormalizedAccountAndInitialSession(t *testing.T) {
	t.Parallel()

	accountStore, _, _, tokenCodec, refreshGenerator, deps := dependencies(t)
	module := auth.New(deps)

	pair, err := module.Register(context.Background(), auth.RegisterInput{
		Identifier:  " Alice@Example.COM ",
		DisplayName: "Alice",
		Password:    "Secret1!",
	})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	if len(accountStore.created) != 1 {
		t.Fatalf("created registrations = %d, want 1", len(accountStore.created))
	}
	registration := accountStore.created[0]
	if registration.Identifier != "alice@example.com" {
		t.Errorf("stored identifier = %q, want canonical lower case", registration.Identifier)
	}
	if registration.AccountID != "01ARZ3NDEKTSV4RRFFQ69G5FA1" || registration.SessionID != "01ARZ3NDEKTSV4RRFFQ69G5FA2" {
		t.Errorf("ids = %q/%q", registration.AccountID, registration.SessionID)
	}
	if registration.PasswordHash != "encoded-hash" {
		t.Errorf("password hash = %q", registration.PasswordHash)
	}
	if registration.RefreshTokenHash != auth.HashRefreshToken("refresh-token") {
		t.Errorf("refresh hash = %q", registration.RefreshTokenHash)
	}
	if !registration.SessionExpiresAt.Equal(fixedTime.Add(auth.SessionTTL)) {
		t.Errorf("session expiry = %v, want %v", registration.SessionExpiresAt, fixedTime.Add(auth.SessionTTL))
	}

	if pair.RefreshToken != "refresh-token" || pair.SessionID != registration.SessionID {
		t.Errorf("pair = %+v", pair)
	}
	if pair.AccessToken != "signed-token" {
		t.Errorf("access token = %q", pair.AccessToken)
	}
	if tokenCodec.issued.AccountID != registration.AccountID || tokenCodec.issued.SessionID != registration.SessionID {
		t.Errorf("issued claims = %+v", tokenCodec.issued)
	}
	if pair.RefreshExpiresAt != registration.SessionExpiresAt {
		t.Errorf("response refresh expiry = %v, want session expiry", pair.RefreshExpiresAt)
	}
	if refreshGenerator.token == "" {
		t.Error("refresh generator was not used")
	}
}

func TestRegisterRejectsPolicyBeforePersistence(t *testing.T) {
	t.Parallel()

	accountStore, _, _, _, _, deps := dependencies(t)
	module := auth.New(deps)

	_, err := module.Register(context.Background(), auth.RegisterInput{
		Identifier:  "alice",
		DisplayName: "Alice",
		Password:    "weak",
	})
	var policy *auth.PasswordPolicyError
	if !errors.As(err, &policy) {
		t.Fatalf("error = %v, want PasswordPolicyError", err)
	}
	if len(accountStore.created) != 0 {
		t.Fatal("invalid registration reached the repository")
	}
}

func TestRegisterDoesNotPersistWhenTokenIssueFails(t *testing.T) {
	t.Parallel()

	accountStore, _, _, tokenCodec, _, deps := dependencies(t)
	tokenCodec.issueErr = errors.New("signing failed")
	module := auth.New(deps)

	if _, err := module.Register(context.Background(), auth.RegisterInput{
		Identifier:  "alice",
		DisplayName: "Alice",
		Password:    "Secret1!",
	}); err == nil {
		t.Fatal("Register succeeded, want token error")
	}
	if len(accountStore.created) != 0 {
		t.Fatal("registration was persisted without a token pair")
	}
}

func TestRegisterMapsIdentifierConflict(t *testing.T) {
	t.Parallel()

	accountStore, _, _, _, _, deps := dependencies(t)
	accountStore.err = auth.ErrIdentifierTaken
	module := auth.New(deps)

	_, err := module.Register(context.Background(), auth.RegisterInput{
		Identifier:  "alice",
		DisplayName: "Alice",
		Password:    "Secret1!",
	})
	if !errors.Is(err, auth.ErrIdentifierTaken) {
		t.Fatalf("error = %v, want ErrIdentifierTaken", err)
	}
}

func TestRefreshRotatesAndReturnsPair(t *testing.T) {
	t.Parallel()

	accountStore, sessionStore, _, tokenCodec, refreshGenerator, deps := dependencies(t)
	module := auth.New(deps)
	sessionStore.result = auth.RotationResult{
		Status:    auth.RotationSucceeded,
		SessionID: "01ARZ3NDEKTSV4RRFFQ69G5FA2",
		Profile: auth.Profile{
			ID:          "01ARZ3NDEKTSV4RRFFQ69G5FA1",
			Identifier:  "alice",
			DisplayName: "Alice",
		},
	}

	pair, err := module.Refresh(context.Background(), "old-refresh")
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}

	if sessionStore.rotation.TokenHash != auth.HashRefreshToken("old-refresh") {
		t.Errorf("rotation token hash = %q", sessionStore.rotation.TokenHash)
	}
	if sessionStore.rotation.NewTokenHash != auth.HashRefreshToken(refreshGenerator.token) {
		t.Errorf("new token hash = %q", sessionStore.rotation.NewTokenHash)
	}
	if !sessionStore.rotation.Now.Equal(fixedTime) {
		t.Errorf("rotation time = %v, want %v", sessionStore.rotation.Now, fixedTime)
	}
	if !sessionStore.rotation.SessionExpiresAt.Equal(fixedTime.Add(auth.SessionTTL)) {
		t.Errorf("session expiry = %v", sessionStore.rotation.SessionExpiresAt)
	}
	if pair.SessionID != "01ARZ3NDEKTSV4RRFFQ69G5FA2" || pair.Profile.Identifier != "alice" {
		t.Errorf("pair = %+v", pair)
	}
	if tokenCodec.issued.SessionID != pair.SessionID || tokenCodec.issued.AccountID != pair.Profile.ID {
		t.Errorf("access claims = %+v", tokenCodec.issued)
	}
	if pair.RefreshToken != refreshGenerator.token {
		t.Errorf("refresh token = %q", pair.RefreshToken)
	}
	if accountStore.created != nil {
		t.Error("refresh created a new account")
	}
}

func TestRefreshRejectsReplayAndUnknownTokens(t *testing.T) {
	t.Parallel()

	for _, status := range []auth.RotationStatus{auth.RotationReplay, auth.RotationRejected} {
		sessionStore := &sessions{result: auth.RotationResult{Status: status}}
		module := auth.New(auth.Deps{
			Sessions:      sessionStore,
			Tokens:        &tokens{},
			RefreshTokens: &refreshTokens{},
			Clock:         clock.NewManual(fixedTime),
		})

		if _, err := module.Refresh(context.Background(), "token"); !errors.Is(err, auth.ErrUnauthenticated) {
			t.Errorf("status %v: error = %v, want unauthenticated", status, err)
		}
	}
}

func TestRefreshPropagatesStorageFailure(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("database unavailable")
	module := auth.New(auth.Deps{
		Sessions:      &sessions{err: sentinel},
		Tokens:        &tokens{},
		RefreshTokens: &refreshTokens{},
		Clock:         clock.NewManual(fixedTime),
	})

	if _, err := module.Refresh(context.Background(), "token"); !errors.Is(err, sentinel) {
		t.Fatalf("error = %v, want storage failure", err)
	}
}

func TestGetMeMapsMissingProfileToUnauthenticated(t *testing.T) {
	t.Parallel()

	accountStore := &accounts{err: auth.ErrProfileNotFound}
	module := auth.New(auth.Deps{Accounts: accountStore})

	if _, err := module.GetMe(context.Background(), auth.Claims{AccountID: "account"}); !errors.Is(err, auth.ErrUnauthenticated) {
		t.Fatalf("error = %v, want unauthenticated", err)
	}
}

func TestLogoutPassesCurrentAndAllSessions(t *testing.T) {
	t.Parallel()

	sessionStore := &sessions{}
	module := auth.New(auth.Deps{
		Sessions: sessionStore,
		Clock:    clock.NewManual(fixedTime),
	})
	claims := auth.Claims{AccountID: "account", SessionID: "session"}

	if err := module.Logout(context.Background(), claims, false); err != nil {
		t.Fatalf("Logout current: %v", err)
	}
	if sessionStore.revoked.All || sessionStore.revoked.SessionID != "session" || sessionStore.revoked.AccountID != "account" {
		t.Errorf("current revoke = %+v", sessionStore.revoked)
	}

	if err := module.Logout(context.Background(), claims, true); err != nil {
		t.Fatalf("Logout all: %v", err)
	}
	if !sessionStore.revoked.All || sessionStore.revoked.AccountID != "account" {
		t.Errorf("all revoke = %+v", sessionStore.revoked)
	}
}

func TestAuthenticateRequiresValidatedClaims(t *testing.T) {
	t.Parallel()

	tokenCodec := &tokens{validated: auth.Claims{AccountID: "account", SessionID: "session"}}
	module := auth.New(auth.Deps{Tokens: tokenCodec})

	claims, err := module.Authenticate(context.Background(), "token")
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	if claims.AccountID != "account" || claims.SessionID != "session" {
		t.Errorf("claims = %+v", claims)
	}

	tokenCodec.validated = auth.Claims{AccountID: "account"}
	if _, err := module.Authenticate(context.Background(), "token"); !errors.Is(err, auth.ErrUnauthenticated) {
		t.Fatalf("missing session claim error = %v, want unauthenticated", err)
	}
	if _, err := module.Authenticate(context.Background(), "   "); !errors.Is(err, auth.ErrUnauthenticated) {
		t.Fatalf("blank token error = %v, want unauthenticated", err)
	}
}
