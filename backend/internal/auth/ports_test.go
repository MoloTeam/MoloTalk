package auth_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/MoloTeam/MoloTalk/backend/internal/auth"
	"github.com/MoloTeam/MoloTalk/backend/internal/platform/clock"
	"github.com/MoloTeam/MoloTalk/backend/internal/platform/idgen"
)

type accounts struct {
	created []auth.Registration
	profile auth.Profile
	err     error
}

func (f *accounts) CreateWithSession(_ context.Context, registration auth.Registration) error {
	f.created = append(f.created, registration)
	return f.err
}

func (f *accounts) Profile(context.Context, string) (auth.Profile, error) {
	return f.profile, f.err
}

type sessions struct {
	rotation auth.Rotation
	result   auth.RotationResult
	revoked  auth.RevokeRequest
	err      error
}

func (f *sessions) RotateRefresh(ctx context.Context, rotation auth.Rotation, issue auth.IssueAccess) (auth.RotationResult, error) {
	f.rotation = rotation
	result := f.result
	if f.err != nil || result.Status != auth.RotationSucceeded {
		return result, f.err
	}
	var err error
	result.Access, err = issue(ctx, auth.Claims{AccountID: result.Profile.ID, SessionID: result.SessionID})
	return result, err
}

func (f *sessions) RevokeSessions(_ context.Context, request auth.RevokeRequest) error {
	f.revoked = request
	return f.err
}

type hasher struct {
	hash       string
	verified   bool
	hashErr    error
	verifyErr  error
	lastHash   string
	lastVerify string
}

func (f *hasher) Hash(context.Context, string) (string, error) {
	if f.hash == "" {
		f.hash = "encoded-hash"
	}
	return f.hash, f.hashErr
}

func (f *hasher) Verify(_ context.Context, password, hash string) (bool, error) {
	f.lastVerify = password + "|" + hash
	return f.verified, f.verifyErr
}

type tokens struct {
	issued      auth.Claims
	validated   auth.Claims
	issueErr    error
	validateErr error
	expires     time.Time
}

func (f *tokens) Issue(_ context.Context, claims auth.Claims) (auth.AccessToken, error) {
	f.issued = claims
	expires := f.expires
	if expires.IsZero() {
		expires = time.Unix(1000, 0).UTC()
	}
	return auth.AccessToken{Value: "signed-token", ExpiresAt: expires}, f.issueErr
}

func (f *tokens) Validate(context.Context, string) (auth.Claims, error) {
	return f.validated, f.validateErr
}

type refreshTokens struct {
	token string
	hash  string
	err   error
}

func (f *refreshTokens) New() (string, string, error) {
	if f.token == "" {
		f.token = "refresh-token"
	}
	if f.hash == "" {
		f.hash = auth.HashRefreshToken(f.token)
	}
	return f.token, f.hash, f.err
}

var (
	_ auth.AccountRepository = (*accounts)(nil)
	_ auth.SessionRepository = (*sessions)(nil)
	_ auth.PasswordHasher    = (*hasher)(nil)
	_ auth.TokenCodec        = (*tokens)(nil)
	_ auth.RefreshTokens     = (*refreshTokens)(nil)
)

func TestAuthPortsExposeTheContract(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("storage unavailable")
	module := auth.New(auth.Deps{
		Accounts:      &accounts{err: sentinel},
		Sessions:      &sessions{err: sentinel},
		Hasher:        &hasher{hashErr: sentinel},
		Tokens:        &tokens{validateErr: sentinel},
		RefreshTokens: &refreshTokens{err: sentinel},
		Clock:         clock.NewManual(time.Unix(0, 0)),
		IDs:           idgen.NewSequence(),
	})

	if module == nil {
		t.Fatal("module was not constructed")
	}
	if module.Name() != "auth" {
		t.Errorf("name = %q, want auth", module.Name())
	}
	if checks := module.Readiness(); len(checks) != 0 {
		t.Errorf("readiness checks = %d, want the empty framework set", len(checks))
	}

	if _, err := module.Authenticate(context.Background(), "broken"); !errors.Is(err, auth.ErrUnauthenticated) {
		t.Errorf("Authenticate error = %v, want unauthenticated", err)
	}
}
