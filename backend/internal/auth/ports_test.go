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

type accounts struct{ err error }

func (f accounts) FindByIdentifier(context.Context, string) (auth.Account, error) {
	return auth.Account{}, f.err
}

func (f accounts) Create(context.Context, auth.Account) error { return f.err }

func (f accounts) Update(context.Context, auth.Account) error { return f.err }

type sessions struct{ err error }

func (f sessions) ConsumeRefresh(context.Context, string) (auth.Session, error) {
	return auth.Session{}, f.err
}

func (f sessions) Revoke(context.Context, string) error { return f.err }

type hasher struct{ err error }

func (f hasher) Hash(context.Context, string) (string, error) { return "", f.err }

func (f hasher) Verify(context.Context, string, string) (bool, error) { return false, f.err }

type tokens struct{ err error }

func (f tokens) Issue(context.Context, auth.Claims) (string, error) { return "", f.err }

func (f tokens) Validate(context.Context, string) (auth.Claims, error) {
	return auth.Claims{}, f.err
}

var (
	_ auth.AccountRepository = accounts{}
	_ auth.SessionRepository = sessions{}
	_ auth.PasswordHasher    = hasher{}
	_ auth.TokenCodec        = tokens{}
)

func TestAuthPortsExposeTheContract(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("storage unavailable")
	module := auth.New(auth.Deps{
		Accounts: accounts{err: sentinel},
		Sessions: sessions{err: sentinel},
		Hasher:   hasher{err: sentinel},
		Tokens:   tokens{err: sentinel},
		Clock:    clock.NewManual(time.Unix(0, 0)),
		IDs:      idgen.NewSequence(),
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

	ctx := context.Background()
	var repository auth.AccountRepository = accounts{err: sentinel}
	if _, err := repository.FindByIdentifier(ctx, "user@example.com"); !errors.Is(err, sentinel) {
		t.Errorf("FindByIdentifier error = %v, want the port failure", err)
	}

	var codec auth.TokenCodec = tokens{err: sentinel}
	if _, err := codec.Issue(ctx, auth.Claims{AccountID: "account"}); !errors.Is(err, sentinel) {
		t.Errorf("Issue error = %v, want the port failure", err)
	}
}
