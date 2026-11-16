// Package auth объявляет порты аккаунтов и сеансов и собирает модуль Auth.
package auth

import (
	"context"

	"github.com/MoloTeam/MoloTalk/backend/internal/platform/clock"
	"github.com/MoloTeam/MoloTalk/backend/internal/platform/httpserver"
	"github.com/MoloTeam/MoloTalk/backend/internal/platform/idgen"
)

// Account — учётная запись пользователя.
type Account struct {
	ID           string
	Identifier   string
	PasswordHash string
}

// Session — сеанс обновления токена.
type Session struct {
	ID           string
	AccountID    string
	RefreshToken string
}

// Claims — проверенные данные токена.
type Claims struct {
	AccountID string
}

// AccountRepository хранит учётные записи.
type AccountRepository interface {
	FindByIdentifier(ctx context.Context, identifier string) (Account, error)
	Create(ctx context.Context, account Account) error
	Update(ctx context.Context, account Account) error
}

// SessionRepository хранит сеансы обновления.
type SessionRepository interface {
	ConsumeRefresh(ctx context.Context, refreshToken string) (Session, error)
	Revoke(ctx context.Context, sessionID string) error
}

// PasswordHasher хеширует и проверяет пароли.
type PasswordHasher interface {
	Hash(ctx context.Context, password string) (string, error)
	Verify(ctx context.Context, password, hash string) (bool, error)
}

// TokenCodec выдаёт и проверяет токены.
type TokenCodec interface {
	Issue(ctx context.Context, claims Claims) (string, error)
	Validate(ctx context.Context, token string) (Claims, error)
}

// Deps задаёт зависимости модуля Auth.
type Deps struct {
	Accounts AccountRepository
	Sessions SessionRepository
	Hasher   PasswordHasher
	Tokens   TokenCodec
	Clock    clock.Clock
	IDs      idgen.IDGenerator
}

// Module — прикладной модуль Auth.
type Module struct {
	deps Deps
}

// New собирает модуль с переданными зависимостями.
func New(deps Deps) *Module {
	return &Module{deps: deps}
}

// Name возвращает имя процесса.
func (m *Module) Name() string {
	return "auth"
}

// Readiness возвращает проверки готовности модуля. В каркасе T-01 набор пуст.
func (m *Module) Readiness() []httpserver.ReadinessCheck {
	return nil
}
