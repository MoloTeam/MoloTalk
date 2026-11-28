package refresh_test

import (
	"bytes"
	"encoding/base64"
	"strings"
	"testing"

	"github.com/MoloTeam/MoloTalk/backend/internal/auth"
	"github.com/MoloTeam/MoloTalk/backend/internal/auth/refresh"
)

func TestNewReturnsOpaqueTokenAndDeterministicHash(t *testing.T) {
	t.Parallel()

	entropy := bytes.Repeat([]byte{0x7f}, 32)
	token, hash, err := refresh.NewWithReader(bytes.NewReader(entropy)).New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	decoded, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		t.Fatalf("token is not unpadded base64url: %v", err)
	}
	if len(decoded) != 32 {
		t.Errorf("token entropy = %d bytes, want 32", len(decoded))
	}
	if token == hash {
		t.Fatal("token equals its hash")
	}
	if hash != auth.HashRefreshToken(token) {
		t.Errorf("hash = %q, want SHA-256 of the token", hash)
	}
	if strings.Contains(hash, token) {
		t.Fatal("hash contains the raw token")
	}
}

func TestSameEntropyProducesSameToken(t *testing.T) {
	t.Parallel()

	entropy := bytes.Repeat([]byte{0x11}, 32)
	first, firstHash, err := refresh.NewWithReader(bytes.NewReader(entropy)).New()
	if err != nil {
		t.Fatalf("New first: %v", err)
	}
	second, secondHash, err := refresh.NewWithReader(bytes.NewReader(entropy)).New()
	if err != nil {
		t.Fatalf("New second: %v", err)
	}
	if first != second || firstHash != secondHash {
		t.Fatalf("same entropy produced %q/%q and %q/%q", first, firstHash, second, secondHash)
	}
}

func TestDifferentEntropyProducesDifferentTokens(t *testing.T) {
	t.Parallel()

	first, _, err := refresh.NewWithReader(bytes.NewReader(bytes.Repeat([]byte{0x01}, 32))).New()
	if err != nil {
		t.Fatalf("New first: %v", err)
	}
	second, _, err := refresh.NewWithReader(bytes.NewReader(bytes.Repeat([]byte{0x02}, 32))).New()
	if err != nil {
		t.Fatalf("New second: %v", err)
	}
	if first == second {
		t.Fatal("different entropy produced the same token")
	}
}

func TestNewPropagatesEntropyFailure(t *testing.T) {
	t.Parallel()

	if _, _, err := refresh.NewWithReader(bytes.NewReader(nil)).New(); err == nil {
		t.Fatal("New succeeded with an exhausted entropy source")
	}
}
