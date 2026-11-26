package authrpc_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect/v2"
	"github.com/oklog/ulid/v2"

	"github.com/MoloTeam/MoloTalk/backend/internal/auth"
	"github.com/MoloTeam/MoloTalk/backend/internal/auth/authrpc"
	"github.com/MoloTeam/MoloTalk/backend/internal/platform/clock"
	"github.com/MoloTeam/MoloTalk/backend/internal/platform/idgen"
)

const strongPassword = "Str0ng!Pass"

type memoryAccounts struct {
	mu    sync.Mutex
	items map[string]auth.Account
}

func (m *memoryAccounts) FindByIdentifier(_ context.Context, identifier string) (auth.Account, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	account, ok := m.items[identifier]
	if !ok {
		return auth.Account{}, auth.ErrAccountNotFound
	}
	return account, nil
}

func (m *memoryAccounts) Create(_ context.Context, account auth.Account) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.items[account.Identifier]; exists {
		return auth.ErrIdentifierTaken
	}
	m.items[account.Identifier] = account
	return nil
}

func (m *memoryAccounts) Update(context.Context, auth.Account) error { return nil }

func (m *memoryAccounts) count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.items)
}

type fixedHasher struct{}

func (fixedHasher) Hash(context.Context, string) (string, error) { return "hash$salt$digest", nil }

func (fixedHasher) Verify(context.Context, string, string) (bool, error) { return true, nil }

func newModule() (*auth.Module, *memoryAccounts) {
	accounts := &memoryAccounts{items: map[string]auth.Account{}}
	var first, second ulid.ULID
	first[15] = 1
	second[15] = 2
	module := auth.New(auth.Deps{
		Accounts: accounts,
		Hasher:   fixedHasher{},
		Clock:    clock.NewManual(time.Unix(0, 0)),
		IDs:      idgen.NewSequence(first, second),
	})
	return module, accounts
}

func register(module *auth.Module, identifier, password string) error {
	_, err := module.Register(context.Background(), auth.RegisterInput{
		Identifier:  identifier,
		DisplayName: "Alice",
		Password:    password,
	})
	return authrpc.Error(err)
}

func TestInvalidPasswordIsInvalidArgumentAndCreatesNoAccount(t *testing.T) {
	t.Parallel()

	passwords := []string{
		"Aa1!x",
		"Aa1!" + strings.Repeat("x", 61),
		"Aa1! xx",
		"Aa1!жxx",
		"Aa1!😀xx",
		"Aa1!\x07xx",
		"Aa1!~xx",
		"aa1!xxx",
		"AA1!XXX",
		"Aaa!xxx",
		"Aa1xxxx",
	}
	for _, password := range passwords {
		module, accounts := newModule()

		err := register(module, "alice", password)

		if connect.CodeOf(err) != connect.CodeInvalidArgument {
			t.Errorf("password %q: code = %v, want invalid_argument", password, connect.CodeOf(err))
			continue
		}
		if strings.Contains(err.Error(), password) {
			t.Errorf("password %q: error text contains the password", password)
		}
		if accounts.count() != 0 {
			t.Errorf("password %q: an account was created", password)
		}
	}
}

func TestInvalidPasswordMessageNamesTheRequirements(t *testing.T) {
	t.Parallel()

	module, _ := newModule()
	err := register(module, "alice", "ж")

	var connectErr *connect.Error
	if !errors.As(err, &connectErr) {
		t.Fatalf("error %v is not a connect.Error", err)
	}
	message := err.Error()
	for _, fragment := range []string{"от 6 до 64", "A–Z", "заглавной"} {
		if !strings.Contains(message, fragment) {
			t.Errorf("message %q does not contain %q", message, fragment)
		}
	}
}

func TestInvalidIdentifierIsInvalidArgument(t *testing.T) {
	t.Parallel()

	module, _ := newModule()
	if code := connect.CodeOf(register(module, "   ", strongPassword)); code != connect.CodeInvalidArgument {
		t.Errorf("code = %v, want invalid_argument", code)
	}
}

func TestTakenIdentifierIsAlreadyExists(t *testing.T) {
	t.Parallel()

	module, accounts := newModule()
	if err := register(module, "alice@example.com", strongPassword); err != nil {
		t.Fatalf("first registration: %v", err)
	}

	err := register(module, "Alice@Example.com", strongPassword)

	if connect.CodeOf(err) != connect.CodeAlreadyExists {
		t.Errorf("code = %v, want already_exists", connect.CodeOf(err))
	}
	if accounts.count() != 1 {
		t.Errorf("accounts = %d, want 1", accounts.count())
	}
}

func TestSuccessIsNil(t *testing.T) {
	t.Parallel()

	module, accounts := newModule()
	if err := register(module, "alice", strongPassword); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if accounts.count() != 1 {
		t.Errorf("accounts = %d, want 1", accounts.count())
	}
	if authrpc.Error(nil) != nil {
		t.Error("Error(nil) must be nil")
	}
}

func TestUnknownErrorsPassThroughForTheErrorInterceptor(t *testing.T) {
	t.Parallel()

	failure := errors.New("database is down")
	got := authrpc.Error(failure)
	if !errors.Is(got, failure) {
		t.Fatalf("error = %v, want the original failure", got)
	}
	var connectErr *connect.Error
	if errors.As(got, &connectErr) {
		t.Error("an infrastructure failure must not become a client-facing connect error here")
	}
}

func TestExistingConnectErrorIsKept(t *testing.T) {
	t.Parallel()

	original := connect.NewError(connect.CodePermissionDenied, "denied")
	if got := authrpc.Error(original); connect.CodeOf(got) != connect.CodePermissionDenied {
		t.Errorf("code = %v, want permission_denied", connect.CodeOf(got))
	}
}
