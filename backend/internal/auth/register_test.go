package auth_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/oklog/ulid/v2"

	"github.com/MoloTeam/MoloTalk/backend/internal/auth"
	"github.com/MoloTeam/MoloTalk/backend/internal/platform/clock"
	"github.com/MoloTeam/MoloTalk/backend/internal/platform/idgen"
)

const (
	testPassword    = "Str0ng!Pass"
	secretPassword  = "Zq9!uniqueSecretValue"
	registrationDay = "2026-11-26T12:00:00Z"
)

// memoryAccounts повторяет контракт AccountRepository: Create отклоняет
// повторный идентификатор, поиск без записи возвращает ErrAccountNotFound.
type memoryAccounts struct {
	mu          sync.Mutex
	byID        map[string]auth.Account
	findErr     error
	createErr   error
	findCalls   int
	createCalls int
}

func newMemoryAccounts() *memoryAccounts {
	return &memoryAccounts{byID: map[string]auth.Account{}}
}

func (m *memoryAccounts) FindByIdentifier(_ context.Context, identifier string) (auth.Account, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.findCalls++
	if m.findErr != nil {
		return auth.Account{}, m.findErr
	}
	account, ok := m.byID[identifier]
	if !ok {
		return auth.Account{}, auth.ErrAccountNotFound
	}
	return account, nil
}

func (m *memoryAccounts) Create(_ context.Context, account auth.Account) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.createCalls++
	if m.createErr != nil {
		return m.createErr
	}
	if _, exists := m.byID[account.Identifier]; exists {
		return auth.ErrIdentifierTaken
	}
	m.byID[account.Identifier] = account
	return nil
}

func (m *memoryAccounts) Update(context.Context, auth.Account) error { return nil }

func (m *memoryAccounts) stored() []auth.Account {
	m.mu.Lock()
	defer m.mu.Unlock()

	result := make([]auth.Account, 0, len(m.byID))
	for _, account := range m.byID {
		result = append(result, account)
	}
	return result
}

// recordingHasher имитирует хешер с солью: у каждого вызова своя соль, поэтому
// хеш одного пароля не повторяется, а пароль из хеша не извлекается.
type recordingHasher struct {
	mu        sync.Mutex
	err       error
	override  string
	passwords []string
}

func (h *recordingHasher) Hash(_ context.Context, password string) (string, error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.passwords = append(h.passwords, password)
	if h.err != nil {
		return "", h.err
	}
	if h.override != "" {
		return h.override, nil
	}
	salt := fmt.Sprintf("salt%d", len(h.passwords))
	sum := sha256.Sum256([]byte(salt + password))
	return "test$" + salt + "$" + hex.EncodeToString(sum[:]), nil
}

func (h *recordingHasher) Verify(context.Context, string, string) (bool, error) { return false, nil }

func (h *recordingHasher) calls() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.passwords)
}

type registerEnv struct {
	accounts *memoryAccounts
	hasher   *recordingHasher
	module   *auth.Module
	now      time.Time
}

func testID(n byte) ulid.ULID {
	var id ulid.ULID
	id[15] = n
	return id
}

func newRegisterEnv(t *testing.T) *registerEnv {
	t.Helper()
	return newRegisterEnvWithIDs(t, idgen.NewSequence(testID(1), testID(2), testID(3)))
}

func newRegisterEnvWithIDs(t *testing.T, ids idgen.IDGenerator) *registerEnv {
	t.Helper()

	now, err := time.Parse(time.RFC3339, registrationDay)
	if err != nil {
		t.Fatalf("parse time: %v", err)
	}
	env := &registerEnv{
		accounts: newMemoryAccounts(),
		hasher:   &recordingHasher{},
		now:      now,
	}
	env.module = auth.New(auth.Deps{
		Accounts: env.accounts,
		Hasher:   env.hasher,
		Clock:    clock.NewManual(now),
		IDs:      ids,
	})
	return env
}

func validInput() auth.RegisterInput {
	return auth.RegisterInput{Identifier: "alice@example.com", DisplayName: "Alice", Password: testPassword}
}

func TestRegisterCreatesAccount(t *testing.T) {
	t.Parallel()

	env := newRegisterEnv(t)
	account, err := env.module.Register(context.Background(), validInput())
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	if account.ID != testID(1).String() {
		t.Errorf("ID = %q, want %q", account.ID, testID(1).String())
	}
	if account.Identifier != "alice@example.com" || account.DisplayName != "Alice" {
		t.Errorf("account = %+v", account)
	}
	if !account.CreatedAt.Equal(env.now) || !account.PasswordChangedAt.Equal(env.now) {
		t.Errorf("times = %v / %v, want %v", account.CreatedAt, account.PasswordChangedAt, env.now)
	}
	if account.PasswordHash != "" {
		t.Errorf("returned account exposes the password hash")
	}

	stored := env.accounts.stored()
	if len(stored) != 1 {
		t.Fatalf("stored accounts = %d, want 1", len(stored))
	}
	if stored[0].ID != account.ID || stored[0].Identifier != account.Identifier {
		t.Errorf("stored = %+v, want the registered account", stored[0])
	}
}

func TestRegisterStoresSaltedHashInsteadOfPassword(t *testing.T) {
	t.Parallel()

	env := newRegisterEnv(t)
	if _, err := env.module.Register(context.Background(), validInput()); err != nil {
		t.Fatalf("Register: %v", err)
	}

	if !slices.Equal(env.hasher.passwords, []string{testPassword}) {
		t.Fatalf("hasher received %v, want exactly the submitted password once", env.hasher.passwords)
	}
	stored := env.accounts.stored()[0]
	if stored.PasswordHash == "" || !strings.HasPrefix(stored.PasswordHash, "test$salt1$") {
		t.Errorf("stored hash = %q, want the hasher output", stored.PasswordHash)
	}
	if strings.Contains(stored.PasswordHash, testPassword) {
		t.Errorf("stored hash contains the password")
	}
	if stored.PasswordHash == testPassword {
		t.Errorf("password stored in plain text")
	}
}

func TestRegisterRejectsHasherReturningPlaintext(t *testing.T) {
	t.Parallel()

	env := newRegisterEnv(t)
	env.hasher.override = testPassword

	if _, err := env.module.Register(context.Background(), validInput()); err == nil {
		t.Fatal("Register succeeded although the hasher returned the password as is")
	}
	if env.accounts.createCalls != 0 {
		t.Errorf("Create was called %d times, want 0", env.accounts.createCalls)
	}
}

func TestRegisterRejectsEmptyHash(t *testing.T) {
	t.Parallel()

	env := newRegisterEnv(t)
	env.module = auth.New(auth.Deps{
		Accounts: env.accounts,
		Hasher:   emptyHasher{},
		Clock:    clock.NewManual(env.now),
		IDs:      idgen.NewSequence(testID(1)),
	})

	if _, err := env.module.Register(context.Background(), validInput()); err == nil {
		t.Fatal("Register succeeded with an empty hash")
	}
	if env.accounts.createCalls != 0 {
		t.Errorf("Create was called %d times, want 0", env.accounts.createCalls)
	}
}

type emptyHasher struct{}

func (emptyHasher) Hash(context.Context, string) (string, error) { return "", nil }

func (emptyHasher) Verify(context.Context, string, string) (bool, error) { return false, nil }

func TestRegisterNormalizesIdentifierAndTrimsName(t *testing.T) {
	t.Parallel()

	env := newRegisterEnv(t)
	account, err := env.module.Register(context.Background(), auth.RegisterInput{
		Identifier:  "  Alice@Example.COM \t",
		DisplayName: "  Alice Liddell  ",
		Password:    testPassword,
	})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if account.Identifier != "alice@example.com" {
		t.Errorf("Identifier = %q, want lower-case without spaces", account.Identifier)
	}
	if account.DisplayName != "Alice Liddell" {
		t.Errorf("DisplayName = %q, want trimmed", account.DisplayName)
	}
}

func TestNormalizeIdentifier(t *testing.T) {
	t.Parallel()

	tests := []struct {
		raw  string
		want string
	}{
		{raw: "alice", want: "alice"},
		{raw: "Alice", want: "alice"},
		{raw: "  ALICE  ", want: "alice"},
		{raw: "Alice@Mail.RU", want: "alice@mail.ru"},
		{raw: "\tМария\n", want: "мария"},
		{raw: "already@lower.com", want: "already@lower.com"},
	}
	for _, test := range tests {
		if got := auth.NormalizeIdentifier(test.raw); got != test.want {
			t.Errorf("NormalizeIdentifier(%q) = %q, want %q", test.raw, got, test.want)
		}
	}
}

func TestRegisterIdentifierIsUniqueIgnoringCase(t *testing.T) {
	t.Parallel()

	env := newRegisterEnv(t)
	ctx := context.Background()
	if _, err := env.module.Register(ctx, validInput()); err != nil {
		t.Fatalf("first Register: %v", err)
	}

	for _, identifier := range []string{"alice@example.com", "ALICE@EXAMPLE.COM", " Alice@Example.com "} {
		hashesBefore := env.hasher.calls()
		createsBefore := env.accounts.createCalls

		input := validInput()
		input.Identifier = identifier
		_, err := env.module.Register(ctx, input)
		if !errors.Is(err, auth.ErrIdentifierTaken) {
			t.Errorf("Register(%q) error = %v, want ErrIdentifierTaken", identifier, err)
		}
		if env.hasher.calls() != hashesBefore {
			t.Errorf("Register(%q) hashed a password for a taken identifier", identifier)
		}
		if env.accounts.createCalls != createsBefore {
			t.Errorf("Register(%q) tried to create a duplicate", identifier)
		}
	}
	if got := len(env.accounts.stored()); got != 1 {
		t.Errorf("stored accounts = %d, want 1", got)
	}
}

func TestRegisterReportsIdentifierTakenOnConcurrentCreate(t *testing.T) {
	t.Parallel()

	env := newRegisterEnv(t)
	env.accounts.createErr = fmt.Errorf("unique violation: %w", auth.ErrIdentifierTaken)

	_, err := env.module.Register(context.Background(), validInput())
	if !errors.Is(err, auth.ErrIdentifierTaken) {
		t.Fatalf("error = %v, want ErrIdentifierTaken", err)
	}
}

func TestRegisterRejectsInvalidPasswordWithoutCreatingAccount(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		password string
		want     []auth.PasswordRule
	}{
		{name: "too short", password: padded(5), want: []auth.PasswordRule{auth.PasswordRuleLength}},
		{name: "too long", password: padded(65), want: []auth.PasswordRule{auth.PasswordRuleLength}},
		{name: "space", password: "Aa1! xx", want: []auth.PasswordRule{auth.PasswordRuleCharacters}},
		{name: "cyrillic", password: "Aa1!жxx", want: []auth.PasswordRule{auth.PasswordRuleCharacters}},
		{name: "emoji", password: "Aa1!😀xx", want: []auth.PasswordRule{auth.PasswordRuleCharacters}},
		{name: "control character", password: "Aa1!\x07xx", want: []auth.PasswordRule{auth.PasswordRuleCharacters}},
		{name: "character outside the set", password: "Aa1!~xx", want: []auth.PasswordRule{auth.PasswordRuleCharacters}},
		{name: "no uppercase", password: "aa1!xxx", want: []auth.PasswordRule{auth.PasswordRuleComposition}},
		{name: "no lowercase", password: "AA1!XXX", want: []auth.PasswordRule{auth.PasswordRuleComposition}},
		{name: "no digit", password: "Aaa!xxx", want: []auth.PasswordRule{auth.PasswordRuleComposition}},
		{name: "no special", password: "Aa1xxxx", want: []auth.PasswordRule{auth.PasswordRuleComposition}},
		{
			name:     "several rules",
			password: "ж",
			want:     []auth.PasswordRule{auth.PasswordRuleLength, auth.PasswordRuleCharacters, auth.PasswordRuleComposition},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			env := newRegisterEnv(t)
			input := validInput()
			input.Password = test.password

			_, err := env.module.Register(context.Background(), input)

			var policy *auth.PasswordPolicyError
			if !errors.As(err, &policy) {
				t.Fatalf("error = %v, want *PasswordPolicyError", err)
			}
			if !slices.Equal(policy.Rules(), test.want) {
				t.Errorf("rules = %v, want %v", policy.Rules(), test.want)
			}
			if got := len(env.accounts.stored()); got != 0 {
				t.Errorf("stored accounts = %d, want 0", got)
			}
			if env.accounts.createCalls != 0 || env.hasher.calls() != 0 || env.accounts.findCalls != 0 {
				t.Errorf("dependencies touched: find=%d create=%d hash=%d",
					env.accounts.findCalls, env.accounts.createCalls, env.hasher.calls())
			}
		})
	}
}

func TestRegisterChecksPasswordBeforeUniqueness(t *testing.T) {
	t.Parallel()

	env := newRegisterEnv(t)
	ctx := context.Background()
	if _, err := env.module.Register(ctx, validInput()); err != nil {
		t.Fatalf("first Register: %v", err)
	}

	input := validInput()
	input.Password = "weak"
	_, err := env.module.Register(ctx, input)

	var policy *auth.PasswordPolicyError
	if !errors.As(err, &policy) {
		t.Fatalf("error = %v, want *PasswordPolicyError", err)
	}
}

func TestRegisterValidatesIdentifierAndDisplayName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		input     auth.RegisterInput
		wantField string
	}{
		{name: "empty identifier", input: auth.RegisterInput{Identifier: "", DisplayName: "A", Password: testPassword}, wantField: auth.FieldIdentifier},
		{name: "blank identifier", input: auth.RegisterInput{Identifier: " \t ", DisplayName: "A", Password: testPassword}, wantField: auth.FieldIdentifier},
		{name: "identifier 255 characters", input: auth.RegisterInput{Identifier: strings.Repeat("a", 255), DisplayName: "A", Password: testPassword}, wantField: auth.FieldIdentifier},
		{name: "identifier 255 multibyte characters", input: auth.RegisterInput{Identifier: strings.Repeat("ж", 255), DisplayName: "A", Password: testPassword}, wantField: auth.FieldIdentifier},
		{name: "empty display name", input: auth.RegisterInput{Identifier: "a", DisplayName: "", Password: testPassword}, wantField: auth.FieldDisplayName},
		{name: "blank display name", input: auth.RegisterInput{Identifier: "a", DisplayName: "  \n ", Password: testPassword}, wantField: auth.FieldDisplayName},
		{name: "display name 65 characters", input: auth.RegisterInput{Identifier: "a", DisplayName: strings.Repeat("n", 65), Password: testPassword}, wantField: auth.FieldDisplayName},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			env := newRegisterEnv(t)
			_, err := env.module.Register(context.Background(), test.input)

			var validation *auth.ValidationError
			if !errors.As(err, &validation) {
				t.Fatalf("error = %v, want *ValidationError", err)
			}
			if validation.Field != test.wantField {
				t.Errorf("field = %q, want %q", validation.Field, test.wantField)
			}
			if env.accounts.createCalls != 0 || env.hasher.calls() != 0 {
				t.Errorf("dependencies touched: create=%d hash=%d", env.accounts.createCalls, env.hasher.calls())
			}
		})
	}
}

func TestRegisterAcceptsBoundaryLengths(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input auth.RegisterInput
	}{
		{name: "identifier 254 characters", input: auth.RegisterInput{Identifier: strings.Repeat("a", 254), DisplayName: "A", Password: testPassword}},
		{name: "identifier 254 multibyte characters", input: auth.RegisterInput{Identifier: strings.Repeat("ж", 254), DisplayName: "A", Password: testPassword}},
		{name: "display name 64 characters", input: auth.RegisterInput{Identifier: "a", DisplayName: strings.Repeat("n", 64), Password: testPassword}},
		{name: "password 6 characters", input: auth.RegisterInput{Identifier: "a", DisplayName: "A", Password: padded(6)}},
		{name: "password 64 characters", input: auth.RegisterInput{Identifier: "a", DisplayName: "A", Password: padded(64)}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			env := newRegisterEnv(t)
			if _, err := env.module.Register(context.Background(), test.input); err != nil {
				t.Fatalf("Register: %v", err)
			}
		})
	}
}

func TestRegisterWrapsDependencyFailures(t *testing.T) {
	t.Parallel()

	failure := errors.New("dependency unavailable")

	t.Run("lookup", func(t *testing.T) {
		t.Parallel()
		env := newRegisterEnv(t)
		env.accounts.findErr = failure
		_, err := env.module.Register(context.Background(), validInput())
		if !errors.Is(err, failure) {
			t.Fatalf("error = %v, want the lookup failure", err)
		}
		if env.accounts.createCalls != 0 {
			t.Errorf("Create called after a failed lookup")
		}
	})
	t.Run("hasher", func(t *testing.T) {
		t.Parallel()
		env := newRegisterEnv(t)
		env.hasher.err = failure
		_, err := env.module.Register(context.Background(), validInput())
		if !errors.Is(err, failure) {
			t.Fatalf("error = %v, want the hasher failure", err)
		}
		if env.accounts.createCalls != 0 {
			t.Errorf("Create called after a failed hash")
		}
	})
	t.Run("id generator", func(t *testing.T) {
		t.Parallel()
		env := newRegisterEnvWithIDs(t, idgen.NewSequence())
		_, err := env.module.Register(context.Background(), validInput())
		if !errors.Is(err, idgen.ErrExhausted) {
			t.Fatalf("error = %v, want ErrExhausted", err)
		}
		if env.accounts.createCalls != 0 {
			t.Errorf("Create called without an ID")
		}
	})
	t.Run("create", func(t *testing.T) {
		t.Parallel()
		env := newRegisterEnv(t)
		env.accounts.createErr = failure
		_, err := env.module.Register(context.Background(), validInput())
		if !errors.Is(err, failure) {
			t.Fatalf("error = %v, want the storage failure", err)
		}
		if errors.Is(err, auth.ErrIdentifierTaken) {
			t.Errorf("a storage failure was reported as a taken identifier")
		}
	})
}

func TestRegisterErrorsDoNotContainPassword(t *testing.T) {
	t.Parallel()

	failure := errors.New("dependency unavailable")
	tests := []struct {
		name  string
		setup func(*registerEnv)
		input auth.RegisterInput
	}{
		{name: "policy violation", setup: func(*registerEnv) {}, input: auth.RegisterInput{Identifier: "a", DisplayName: "A", Password: secretPassword + " "}},
		{name: "lookup failure", setup: func(e *registerEnv) { e.accounts.findErr = failure }, input: auth.RegisterInput{Identifier: "a", DisplayName: "A", Password: secretPassword}},
		{name: "hasher failure", setup: func(e *registerEnv) { e.hasher.err = failure }, input: auth.RegisterInput{Identifier: "a", DisplayName: "A", Password: secretPassword}},
		{name: "storage failure", setup: func(e *registerEnv) { e.accounts.createErr = failure }, input: auth.RegisterInput{Identifier: "a", DisplayName: "A", Password: secretPassword}},
		{name: "plaintext hash", setup: func(e *registerEnv) { e.hasher.override = secretPassword }, input: auth.RegisterInput{Identifier: "a", DisplayName: "A", Password: secretPassword}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			env := newRegisterEnv(t)
			test.setup(env)
			_, err := env.module.Register(context.Background(), test.input)
			if err == nil {
				t.Fatal("Register succeeded, want an error")
			}
			for _, text := range []string{err.Error(), fmt.Sprintf("%v", err), fmt.Sprintf("%+v", err), fmt.Sprintf("%#v", err)} {
				if strings.Contains(text, test.input.Password) {
					t.Errorf("error text contains the password: %q", text)
				}
			}
		})
	}
}

func TestRegisterInputHidesPasswordInFormattingAndLogs(t *testing.T) {
	t.Parallel()

	input := auth.RegisterInput{Identifier: "alice", DisplayName: "Alice", Password: secretPassword}

	for _, text := range []string{
		fmt.Sprint(input),
		fmt.Sprintf("%v", input),
		fmt.Sprintf("%+v", input),
		fmt.Sprintf("%#v", input),
		fmt.Sprintf("%s", input),
		fmt.Sprintf("%v", &input),
	} {
		if strings.Contains(text, secretPassword) {
			t.Errorf("formatted input exposes the password: %q", text)
		}
	}

	for name, handler := range map[string]func(*bytes.Buffer) slog.Handler{
		"json": func(buffer *bytes.Buffer) slog.Handler { return slog.NewJSONHandler(buffer, nil) },
		"text": func(buffer *bytes.Buffer) slog.Handler { return slog.NewTextHandler(buffer, nil) },
	} {
		var buffer bytes.Buffer
		slog.New(handler(&buffer)).Info("register", "input", input)
		if strings.Contains(buffer.String(), secretPassword) {
			t.Errorf("%s log exposes the password: %s", name, buffer.String())
		}
		if !strings.Contains(buffer.String(), "REDACTED") {
			t.Errorf("%s log does not mark the redaction: %s", name, buffer.String())
		}
	}
}
