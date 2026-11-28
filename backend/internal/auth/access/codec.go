// Package access выдаёт и локально проверяет access JWT по профилю Ed25519.
package access

import (
	"context"
	"crypto/ed25519"
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/oklog/ulid/v2"

	"github.com/MoloTeam/MoloTalk/backend/internal/auth"
	"github.com/MoloTeam/MoloTalk/backend/internal/platform/clock"
)

const algorithm = "EdDSA"

// Codec подписывает и проверяет access JWT закрытым ключом Auth.
type Codec struct {
	key   ed25519.PrivateKey
	clock clock.Clock
}

// NewCodec создаёт кодек с заданным ключом и управляемыми часами.
func NewCodec(key ed25519.PrivateKey, source clock.Clock) *Codec {
	return &Codec{key: key, clock: source}
}

type tokenClaims struct {
	SessionID string `json:"sid"`
	jwt.RegisteredClaims
}

// Issue создаёт access JWT с sub, sid, iat и exp на 15 минут.
func (c *Codec) Issue(_ context.Context, claims auth.Claims) (auth.AccessToken, error) {
	if len(c.key) != ed25519.PrivateKeySize {
		return auth.AccessToken{}, errors.New("access: signing key is not an Ed25519 private key")
	}
	if _, err := ulid.Parse(claims.AccountID); err != nil {
		return auth.AccessToken{}, errors.New("access: subject must be a ULID")
	}
	if _, err := ulid.Parse(claims.SessionID); err != nil {
		return auth.AccessToken{}, errors.New("access: session id must be a ULID")
	}

	now := c.clock.Now().UTC().Truncate(time.Second)
	expiresAt := now.Add(auth.AccessTokenTTL)
	token := jwt.NewWithClaims(jwt.SigningMethodEdDSA, tokenClaims{
		SessionID: claims.SessionID,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   claims.AccountID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(expiresAt),
		},
	})
	signed, err := token.SignedString(c.key)
	if err != nil {
		return auth.AccessToken{}, errors.New("access: sign token")
	}
	return auth.AccessToken{Value: signed, ExpiresAt: expiresAt}, nil
}

// Validate проверяет подпись, срок и обязательные claims sub, sid и exp.
// Допускается только алгоритм EdDSA; состояние сеанса локально не читается.
func (c *Codec) Validate(_ context.Context, raw string) (auth.Claims, error) {
	parsed, err := jwt.ParseWithClaims(raw, &tokenClaims{}, func(token *jwt.Token) (any, error) {
		if token.Method != jwt.SigningMethodEdDSA {
			return nil, errors.New("access: unexpected signing method")
		}
		return c.key.Public(), nil
	},
		jwt.WithValidMethods([]string{algorithm}),
		jwt.WithExpirationRequired(),
		jwt.WithTimeFunc(c.clock.Now),
	)
	if err != nil || !parsed.Valid {
		return auth.Claims{}, errors.New("access: invalid token")
	}

	claims, ok := parsed.Claims.(*tokenClaims)
	if !ok || claims.Subject == "" || claims.SessionID == "" || claims.ExpiresAt == nil {
		return auth.Claims{}, errors.New("access: missing required claims")
	}
	if _, err := ulid.Parse(claims.Subject); err != nil {
		return auth.Claims{}, errors.New("access: invalid subject claim")
	}
	if _, err := ulid.Parse(claims.SessionID); err != nil {
		return auth.Claims{}, errors.New("access: invalid session claim")
	}

	return auth.Claims{AccountID: claims.Subject, SessionID: claims.SessionID}, nil
}
