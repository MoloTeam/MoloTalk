package auth_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/MoloTeam/MoloTalk/backend/internal/auth"
	"github.com/MoloTeam/MoloTalk/backend/internal/platform/clock"
	"github.com/MoloTeam/MoloTalk/backend/internal/platform/idgen"
	"github.com/MoloTeam/MoloTalk/backend/internal/platform/passwordhash"
)

// Argon2id реализует порт auth.PasswordHasher.
var _ auth.PasswordHasher = (*passwordhash.Argon2id)(nil)

// TestRegisterWithArgon2idStoresVerifiableSaltedHash проверяет NF_Sec_1 на
// настоящем хешере: в хранилище попадает хеш argon2id с солью, а не пароль.
func TestRegisterWithArgon2idStoresVerifiableSaltedHash(t *testing.T) {
	t.Parallel()

	hasher := passwordhash.NewArgon2id(passwordhash.Params{
		MemoryKiB:   64,
		Iterations:  1,
		Parallelism: 1,
		SaltLength:  16,
		KeyLength:   32,
	})
	accounts := newMemoryAccounts()
	module := auth.New(auth.Deps{
		Accounts: accounts,
		Hasher:   hasher,
		Clock:    clock.NewManual(time.Unix(1_800_000_000, 0)),
		IDs:      idgen.NewSequence(testID(1), testID(2)),
	})
	ctx := context.Background()

	for _, identifier := range []string{"alice", "bob"} {
		_, err := module.Register(ctx, auth.RegisterInput{
			Identifier:  identifier,
			DisplayName: strings.ToUpper(identifier),
			Password:    testPassword,
		})
		if err != nil {
			t.Fatalf("Register(%q): %v", identifier, err)
		}
	}

	hashes := map[string]string{}
	for _, account := range accounts.stored() {
		hash := account.PasswordHash
		hashes[account.Identifier] = hash

		if !strings.HasPrefix(hash, "$argon2id$v=19$") {
			t.Errorf("%s: hash %q is not an argon2id hash", account.Identifier, hash)
		}
		if strings.Contains(hash, testPassword) {
			t.Errorf("%s: hash contains the password", account.Identifier)
		}
		ok, err := hasher.Verify(ctx, testPassword, hash)
		if err != nil || !ok {
			t.Errorf("%s: Verify(correct) = %v, %v; want true, nil", account.Identifier, ok, err)
		}
		ok, err = hasher.Verify(ctx, testPassword+"x", hash)
		if err != nil || ok {
			t.Errorf("%s: Verify(wrong) = %v, %v; want false, nil", account.Identifier, ok, err)
		}
	}

	if len(hashes) != 2 {
		t.Fatalf("stored accounts = %d, want 2", len(hashes))
	}
	if hashes["alice"] == hashes["bob"] {
		t.Error("the same password gave identical hashes for two accounts: the salt is not applied")
	}
}
