package access_test

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/MoloTeam/MoloTalk/backend/internal/auth"
	"github.com/MoloTeam/MoloTalk/backend/internal/auth/access"
	"github.com/MoloTeam/MoloTalk/backend/internal/platform/clock"
)

var (
	issuedAt  = time.Date(2026, time.November, 28, 12, 0, 0, 0, time.UTC)
	testClaim = auth.Claims{
		AccountID: "01ARZ3NDEKTSV4RRFFQ69G5FA1",
		SessionID: "01ARZ3NDEKTSV4RRFFQ69G5FA2",
	}
)

func newKey(t *testing.T) ed25519.PrivateKey {
	t.Helper()
	_, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	return private
}

func TestIssueAndValidateAccessToken(t *testing.T) {
	t.Parallel()

	codec := access.NewCodec(newKey(t), clock.NewManual(issuedAt))
	token, err := codec.Issue(context.Background(), testClaim)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}

	if want := issuedAt.Add(auth.AccessTokenTTL); !token.ExpiresAt.Equal(want) {
		t.Errorf("expiry = %v, want %v", token.ExpiresAt, want)
	}

	claims, err := codec.Validate(context.Background(), token.Value)
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if claims != testClaim {
		t.Errorf("claims = %+v, want %+v", claims, testClaim)
	}

	unverified, _, err := jwt.NewParser().ParseUnverified(token.Value, jwt.MapClaims{})
	if err != nil {
		t.Fatalf("parse unverified: %v", err)
	}
	payload, ok := unverified.Claims.(jwt.MapClaims)
	if !ok {
		t.Fatal("claims are not a map")
	}
	if payload["sub"] != testClaim.AccountID || payload["sid"] != testClaim.SessionID {
		t.Errorf("payload = %v", payload)
	}
	exp, ok := payload["exp"].(float64)
	if !ok || time.Unix(int64(exp), 0).UTC() != token.ExpiresAt {
		t.Errorf("exp claim = %v, want %v", payload["exp"], token.ExpiresAt)
	}
}

func TestValidateRejectsExpiredTokenAtExactExpiry(t *testing.T) {
	t.Parallel()

	manual := clock.NewManual(issuedAt)
	codec := access.NewCodec(newKey(t), manual)
	token, err := codec.Issue(context.Background(), testClaim)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}

	manual.Set(token.ExpiresAt.Add(-time.Second))
	if _, err := codec.Validate(context.Background(), token.Value); err != nil {
		t.Fatalf("token valid before exp: %v", err)
	}

	manual.Set(token.ExpiresAt)
	if _, err := codec.Validate(context.Background(), token.Value); err == nil {
		t.Fatal("token accepted at exp")
	}
}

func TestValidateRejectsWrongSignatureAndAlgorithm(t *testing.T) {
	t.Parallel()

	codec := access.NewCodec(newKey(t), clock.NewManual(issuedAt))
	token, err := codec.Issue(context.Background(), testClaim)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}

	other := access.NewCodec(newKey(t), clock.NewManual(issuedAt))
	if _, err := other.Validate(context.Background(), token.Value); err == nil {
		t.Fatal("token accepted with a different key")
	}

	hs256, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": testClaim.AccountID,
		"sid": testClaim.SessionID,
		"exp": issuedAt.Add(time.Hour).Unix(),
	}).SignedString([]byte("not-an-ed25519-key"))
	if err != nil {
		t.Fatalf("sign HS256: %v", err)
	}
	if _, err := codec.Validate(context.Background(), hs256); err == nil {
		t.Fatal("HS256 token accepted")
	}

	if _, err := codec.Validate(context.Background(), "not-a-jwt"); err == nil {
		t.Fatal("malformed token accepted")
	}
}

func TestValidateRequiresClaimsAndExpiry(t *testing.T) {
	t.Parallel()

	key := newKey(t)
	codec := access.NewCodec(key, clock.NewManual(issuedAt))

	cases := []jwt.MapClaims{
		{"sub": testClaim.AccountID, "sid": testClaim.SessionID},
		{"sub": testClaim.AccountID, "exp": issuedAt.Add(time.Hour).Unix()},
		{"sid": testClaim.SessionID, "exp": issuedAt.Add(time.Hour).Unix()},
		{"sub": "not-a-ulid", "sid": testClaim.SessionID, "exp": issuedAt.Add(time.Hour).Unix()},
		{"sub": testClaim.AccountID, "sid": "not-a-ulid", "exp": issuedAt.Add(time.Hour).Unix()},
	}
	for _, claims := range cases {
		signed, err := jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims).SignedString(key)
		if err != nil {
			t.Fatalf("sign claims %v: %v", claims, err)
		}
		if _, err := codec.Validate(context.Background(), signed); err == nil {
			t.Errorf("token accepted with claims %v", claims)
		}
	}
}

func TestIssueRejectsNonULIDClaims(t *testing.T) {
	t.Parallel()

	codec := access.NewCodec(newKey(t), clock.NewManual(issuedAt))
	if _, err := codec.Issue(context.Background(), auth.Claims{AccountID: "account", SessionID: testClaim.SessionID}); err == nil {
		t.Fatal("Issue accepted a non-ULID subject")
	}
}

func TestLoadSigningKey(t *testing.T) {
	t.Parallel()

	key := newKey(t)
	encoded, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatalf("marshal PKCS#8: %v", err)
	}
	path := filepath.Join(t.TempDir(), "signing-key.pem")
	if err := os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: encoded}), 0o600); err != nil {
		t.Fatalf("write key: %v", err)
	}

	loaded, err := access.LoadSigningKey(path)
	if err != nil {
		t.Fatalf("LoadSigningKey: %v", err)
	}
	if !loaded.Equal(key) {
		t.Fatal("loaded key differs from the generated key")
	}

	broken := filepath.Join(t.TempDir(), "broken.pem")
	if err := os.WriteFile(broken, []byte("not a pem"), 0o600); err != nil {
		t.Fatalf("write broken key: %v", err)
	}
	if _, err := access.LoadSigningKey(broken); err == nil {
		t.Fatal("LoadSigningKey accepted a payload without a PEM key")
	}
	if _, err := access.LoadSigningKey(filepath.Join(t.TempDir(), "missing.pem")); err == nil {
		t.Fatal("LoadSigningKey accepted a missing file")
	}
}
