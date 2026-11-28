package password_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/MoloTeam/MoloTalk/backend/internal/auth/password"
)

var testParams = password.Params{
	Memory:      8 * 1024,
	Iterations:  1,
	Parallelism: 1,
	SaltLength:  16,
	KeyLength:   32,
}

func TestHashUsesIndependentSaltsAndVerifies(t *testing.T) {
	t.Parallel()

	hasher := password.New(testParams)
	first, err := hasher.Hash(context.Background(), "Secret1!")
	if err != nil {
		t.Fatalf("Hash first: %v", err)
	}
	second, err := hasher.Hash(context.Background(), "Secret1!")
	if err != nil {
		t.Fatalf("Hash second: %v", err)
	}

	if first == second {
		t.Fatal("two hashes of the same password are identical; salt is not independent")
	}
	if strings.Contains(first, "Secret1!") || strings.Contains(second, "Secret1!") {
		t.Fatal("encoded hash contains the plain password")
	}
	if !strings.HasPrefix(first, "$argon2id$v=19$m=8192,t=1,p=1$") {
		t.Errorf("hash = %q, want the fixed Argon2id profile", first)
	}

	for _, encoded := range []string{first, second} {
		ok, err := hasher.Verify(context.Background(), "Secret1!", encoded)
		if err != nil || !ok {
			t.Errorf("Verify(correct) = %v, %v; want true", ok, err)
		}
		ok, err = hasher.Verify(context.Background(), "Secret1?", encoded)
		if err != nil {
			t.Errorf("Verify(wrong): %v", err)
		}
		if ok {
			t.Errorf("Verify(wrong) accepted the password for %q", encoded)
		}
	}
}

func TestHashIsDeterministicWithFixedSaltSource(t *testing.T) {
	t.Parallel()

	salt := bytes.Repeat([]byte{0x42}, 16)
	first, err := password.NewWithReader(testParams, bytes.NewReader(salt)).Hash(context.Background(), "Secret1!")
	if err != nil {
		t.Fatalf("Hash first: %v", err)
	}
	second, err := password.NewWithReader(testParams, bytes.NewReader(salt)).Hash(context.Background(), "Secret1!")
	if err != nil {
		t.Fatalf("Hash second: %v", err)
	}
	if first != second {
		t.Fatalf("fixed salt produced different hashes: %q != %q", first, second)
	}
}

func TestVerifyRejectsMalformedHashes(t *testing.T) {
	t.Parallel()

	hasher := password.New(testParams)
	for _, encoded := range []string{
		"",
		"plain",
		"$argon2id$v=19$m=x,t=1,p=1$c2FsdA$aGFzaA",
		"$argon2id$v=18$m=8192,t=1,p=1$c2FsdA$aGFzaA",
		"$argon2id$v=19$m=8192,t=1,p=1$not-base64$not-base64",
	} {
		if _, err := hasher.Verify(context.Background(), "Secret1!", encoded); err == nil {
			t.Errorf("Verify(%q) succeeded, want malformed hash error", encoded)
		}
	}
}

func TestDefaultProfileHashesAndVerifies(t *testing.T) {
	hasher := password.New(password.DefaultParams)
	encoded, err := hasher.Hash(context.Background(), "Secret1!")
	if err != nil {
		t.Fatalf("Hash with default profile: %v", err)
	}
	if !strings.HasPrefix(encoded, "$argon2id$v=19$m=65536,t=3,p=2$") {
		t.Errorf("hash = %q, want the 64 MiB profile", encoded)
	}
	ok, err := hasher.Verify(context.Background(), "Secret1!", encoded)
	if err != nil || !ok {
		t.Fatalf("Verify default profile = %v, %v", ok, err)
	}
}

func TestHashRejectsInvalidParams(t *testing.T) {
	t.Parallel()

	hasher := password.New(password.Params{Memory: 1, Iterations: 1, Parallelism: 1, SaltLength: 16, KeyLength: 32})
	if _, err := hasher.Hash(context.Background(), "Secret1!"); err == nil {
		t.Fatal("Hash accepted an invalid memory parameter")
	}
}
