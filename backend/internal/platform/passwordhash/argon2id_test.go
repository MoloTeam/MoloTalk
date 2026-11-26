package passwordhash

import (
	"bytes"
	"context"
	"errors"
	"io"
	"regexp"
	"strings"
	"testing"
)

const testPassword = "Str0ng!Pass-uniqueValue"

// cheapParams делают тесты быстрыми; боевые параметры проверяются отдельно.
func cheapParams() Params {
	return Params{MemoryKiB: 64, Iterations: 1, Parallelism: 1, SaltLength: 16, KeyLength: 32}
}

func newTestHasher() *Argon2id {
	return NewArgon2id(cheapParams())
}

func fixedSalt(value byte) io.Reader {
	return bytes.NewReader(bytes.Repeat([]byte{value}, 64))
}

type failingReader struct{ err error }

func (r failingReader) Read([]byte) (int, error) { return 0, r.err }

func TestHashAndVerifyRoundTrip(t *testing.T) {
	t.Parallel()

	hasher := newTestHasher()
	ctx := context.Background()

	encoded, err := hasher.Hash(ctx, testPassword)
	if err != nil {
		t.Fatalf("Hash: %v", err)
	}

	ok, err := hasher.Verify(ctx, testPassword, encoded)
	if err != nil || !ok {
		t.Fatalf("Verify(correct) = %v, %v; want true, nil", ok, err)
	}

	for _, wrong := range []string{"", "str0ng!pass-uniqueValue", testPassword + " ", testPassword[:len(testPassword)-1]} {
		ok, err := hasher.Verify(ctx, wrong, encoded)
		if err != nil || ok {
			t.Errorf("Verify(%q) = %v, %v; want false, nil", wrong, ok, err)
		}
	}
}

func TestHashSupportsNonAsciiPasswords(t *testing.T) {
	t.Parallel()

	hasher := newTestHasher()
	ctx := context.Background()
	encoded, err := hasher.Hash(ctx, "Пароль-😀-1!")
	if err != nil {
		t.Fatalf("Hash: %v", err)
	}
	if ok, err := hasher.Verify(ctx, "Пароль-😀-1!", encoded); err != nil || !ok {
		t.Fatalf("Verify = %v, %v; want true, nil", ok, err)
	}
}

func TestHashUsesPHCFormatWithoutPassword(t *testing.T) {
	t.Parallel()

	encoded, err := newTestHasher().Hash(context.Background(), testPassword)
	if err != nil {
		t.Fatalf("Hash: %v", err)
	}

	pattern := regexp.MustCompile(`^\$argon2id\$v=19\$m=64,t=1,p=1\$[A-Za-z0-9+/]{22}\$[A-Za-z0-9+/]{43}$`)
	if !pattern.MatchString(encoded) {
		t.Errorf("hash %q does not match the PHC format", encoded)
	}
	if strings.Contains(encoded, testPassword) {
		t.Errorf("hash contains the password")
	}
}

func TestHashUsesFreshSaltForEveryCall(t *testing.T) {
	t.Parallel()

	hasher := newTestHasher()
	ctx := context.Background()
	first, err := hasher.Hash(ctx, testPassword)
	if err != nil {
		t.Fatalf("Hash: %v", err)
	}
	second, err := hasher.Hash(ctx, testPassword)
	if err != nil {
		t.Fatalf("Hash: %v", err)
	}
	if first == second {
		t.Fatal("two hashes of the same password are identical: the salt is not random")
	}
	for _, encoded := range []string{first, second} {
		if ok, err := hasher.Verify(ctx, testPassword, encoded); err != nil || !ok {
			t.Errorf("Verify = %v, %v; want true, nil", ok, err)
		}
	}
}

func TestHashDependsOnSaltAndPasswordOnly(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	sameSaltA, err := newArgon2id(cheapParams(), fixedSalt(1)).Hash(ctx, testPassword)
	if err != nil {
		t.Fatalf("Hash: %v", err)
	}
	sameSaltB, err := newArgon2id(cheapParams(), fixedSalt(1)).Hash(ctx, testPassword)
	if err != nil {
		t.Fatalf("Hash: %v", err)
	}
	otherSalt, err := newArgon2id(cheapParams(), fixedSalt(2)).Hash(ctx, testPassword)
	if err != nil {
		t.Fatalf("Hash: %v", err)
	}
	otherPassword, err := newArgon2id(cheapParams(), fixedSalt(1)).Hash(ctx, testPassword+"x")
	if err != nil {
		t.Fatalf("Hash: %v", err)
	}

	if sameSaltA != sameSaltB {
		t.Error("the same salt and password must give the same hash")
	}
	if sameSaltA == otherSalt {
		t.Error("a different salt must change the hash")
	}
	if sameSaltA == otherPassword {
		t.Error("a different password must change the hash")
	}
}

func TestVerifyUsesParametersStoredInTheHash(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	encoded, err := NewArgon2id(cheapParams()).Hash(ctx, testPassword)
	if err != nil {
		t.Fatalf("Hash: %v", err)
	}

	stronger := NewArgon2id(Params{MemoryKiB: 256, Iterations: 2, Parallelism: 2, SaltLength: 24, KeyLength: 48})
	ok, err := stronger.Verify(ctx, testPassword, encoded)
	if err != nil || !ok {
		t.Fatalf("a hash made with older parameters must still verify: %v, %v", ok, err)
	}
}

func TestNewArgon2idReplacesWeakOrEmptyParameters(t *testing.T) {
	t.Parallel()

	if got := NewArgon2id(Params{}).params; got != DefaultParams() {
		t.Errorf("empty params = %+v, want defaults %+v", got, DefaultParams())
	}

	weak := NewArgon2id(Params{MemoryKiB: 64, Iterations: 1, Parallelism: 1, SaltLength: 1, KeyLength: 1}).params
	if weak.SaltLength < minSaltLength || weak.KeyLength < minKeyLength {
		t.Errorf("weak salt or key length was kept: %+v", weak)
	}
}

func TestDefaultParamsMatchRFC9106SecondRecommendation(t *testing.T) {
	t.Parallel()

	want := Params{MemoryKiB: 64 * 1024, Iterations: 3, Parallelism: 4, SaltLength: 16, KeyLength: 32}
	if got := DefaultParams(); got != want {
		t.Errorf("DefaultParams = %+v, want %+v", got, want)
	}
}

func TestVerifyRejectsMalformedHashes(t *testing.T) {
	t.Parallel()

	hasher := newTestHasher()
	valid, err := hasher.Hash(context.Background(), testPassword)
	if err != nil {
		t.Fatalf("Hash: %v", err)
	}
	parts := strings.Split(valid, "$") // ["", "argon2id", "v=19", params, salt, key]

	replace := func(index int, value string) string {
		changed := append([]string(nil), parts...)
		changed[index] = value
		return strings.Join(changed, "$")
	}

	// 6 символов base64 без дополнения — 4 байта, 11 символов — 8 байт.
	shortSalt := "AAAAAA"
	shortKey := "AAAAAAAAAAA"

	tests := []struct {
		name    string
		encoded string
	}{
		{name: "empty", encoded: ""},
		{name: "plain password", encoded: testPassword},
		{name: "bcrypt style", encoded: "$2a$10$abcdefghijklmnopqrstuuABCDEFGHIJKLMNOPQRSTUVWXYZ01234"},
		{name: "argon2i", encoded: replace(1, "argon2i")},
		{name: "argon2d", encoded: replace(1, "argon2d")},
		{name: "old version", encoded: replace(2, "v=16")},
		{name: "version without prefix", encoded: replace(2, "19")},
		{name: "params missing", encoded: replace(3, "")},
		{name: "params wrong order", encoded: replace(3, "t=1,m=64,p=1")},
		{name: "params too few", encoded: replace(3, "m=64,t=1")},
		{name: "params too many", encoded: replace(3, "m=64,t=1,p=1,x=1")},
		{name: "memory not a number", encoded: replace(3, "m=abc,t=1,p=1")},
		{name: "memory negative", encoded: replace(3, "m=-64,t=1,p=1")},
		{name: "memory zero", encoded: replace(3, "m=0,t=1,p=1")},
		{name: "memory above limit", encoded: replace(3, "m=262145,t=1,p=1")},
		{name: "memory overflows uint32", encoded: replace(3, "m=99999999999,t=1,p=1")},
		{name: "memory below 8 KiB per thread", encoded: replace(3, "m=8,t=1,p=2")},
		{name: "iterations zero", encoded: replace(3, "m=64,t=0,p=1")},
		{name: "iterations above limit", encoded: replace(3, "m=64,t=11,p=1")},
		{name: "threads zero", encoded: replace(3, "m=64,t=1,p=0")},
		{name: "threads above limit", encoded: replace(3, "m=64,t=1,p=17")},
		{name: "salt not base64", encoded: replace(4, "!!!!!!!!!!!!!!!!!!!!!!")},
		{name: "salt padded", encoded: replace(4, parts[4]+"==")},
		{name: "salt too short", encoded: replace(4, shortSalt)},
		{name: "salt empty", encoded: replace(4, "")},
		{name: "key not base64", encoded: replace(5, strings.Repeat("!", 43))},
		{name: "key too short", encoded: replace(5, shortKey)},
		{name: "key empty", encoded: replace(5, "")},
		{name: "extra segment", encoded: valid + "$extra"},
		{name: "trailing separator", encoded: valid + "$"},
		{name: "missing segment", encoded: strings.Join(parts[:5], "$")},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			ok, err := hasher.Verify(context.Background(), testPassword, test.encoded)
			if ok {
				t.Error("Verify accepted a malformed hash")
			}
			if !errors.Is(err, ErrInvalidHash) {
				t.Errorf("error = %v, want ErrInvalidHash", err)
			}
			if err != nil && (strings.Contains(err.Error(), testPassword) || (test.encoded != "" && strings.Contains(err.Error(), test.encoded))) {
				t.Errorf("error text exposes the password or the hash: %q", err.Error())
			}
		})
	}
}

func TestVerifyDetectsTamperedHash(t *testing.T) {
	t.Parallel()

	hasher := newTestHasher()
	ctx := context.Background()
	valid, err := hasher.Hash(ctx, testPassword)
	if err != nil {
		t.Fatalf("Hash: %v", err)
	}
	parts := strings.Split(valid, "$")

	// Подменённый результат и подменённая соль — это не повреждённый формат,
	// а неверный хеш: пароль не проходит проверку.
	flip := func(segment string) string {
		replacement := "A"
		if strings.HasPrefix(segment, "A") {
			replacement = "B"
		}
		return replacement + segment[1:]
	}
	for _, index := range []int{4, 5} {
		changed := append([]string(nil), parts...)
		changed[index] = flip(changed[index])
		ok, err := hasher.Verify(ctx, testPassword, strings.Join(changed, "$"))
		if err != nil || ok {
			t.Errorf("tampered segment %d: Verify = %v, %v; want false, nil", index, ok, err)
		}
	}
}

func TestHashFailsWhenSaltCannotBeRead(t *testing.T) {
	t.Parallel()

	boom := errors.New("entropy source unavailable")
	_, err := newArgon2id(cheapParams(), failingReader{err: boom}).Hash(context.Background(), testPassword)
	if !errors.Is(err, boom) {
		t.Fatalf("error = %v, want the reader failure", err)
	}
	if strings.Contains(err.Error(), testPassword) {
		t.Error("error text contains the password")
	}

	_, err = newArgon2id(cheapParams(), bytes.NewReader([]byte{1, 2, 3})).Hash(context.Background(), testPassword)
	if !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("short salt: error = %v, want io.ErrUnexpectedEOF", err)
	}
}

func TestCancelledContextStopsHashAndVerify(t *testing.T) {
	t.Parallel()

	hasher := newTestHasher()
	valid, err := hasher.Hash(context.Background(), testPassword)
	if err != nil {
		t.Fatalf("Hash: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := hasher.Hash(ctx, testPassword); !errors.Is(err, context.Canceled) {
		t.Errorf("Hash error = %v, want context.Canceled", err)
	}
	if ok, err := hasher.Verify(ctx, testPassword, valid); ok || !errors.Is(err, context.Canceled) {
		t.Errorf("Verify = %v, %v; want false, context.Canceled", ok, err)
	}
}

func TestDefaultParametersProduceAVerifiableHash(t *testing.T) {
	t.Parallel()

	hasher := NewArgon2id(DefaultParams())
	ctx := context.Background()
	encoded, err := hasher.Hash(ctx, testPassword)
	if err != nil {
		t.Fatalf("Hash: %v", err)
	}
	if !strings.HasPrefix(encoded, "$argon2id$v=19$m=65536,t=3,p=4$") {
		t.Errorf("hash %q does not carry the default parameters", encoded)
	}
	if ok, err := hasher.Verify(ctx, testPassword, encoded); err != nil || !ok {
		t.Fatalf("Verify = %v, %v; want true, nil", ok, err)
	}
}
