// Package auth объявляет порты аккаунтов и сеансов и собирает модуль Auth.
package auth

import (
	"context"
	"time"

	"github.com/MoloTeam/MoloTalk/backend/internal/platform/clock"
	"github.com/MoloTeam/MoloTalk/backend/internal/platform/httpserver"
	"github.com/MoloTeam/MoloTalk/backend/internal/platform/idgen"
)

const (
	// AccessTokenTTL — срок действия access JWT.
	AccessTokenTTL = 15 * time.Minute
	// SessionTTL — срок жизни сеанса и refresh token, продлеваемый ротацией.
	SessionTTL = 30 * 24 * time.Hour
)

// Profile — публичные данные собственного профиля.
type Profile struct {
	ID                 string
	Identifier         string
	DisplayName        string
	AvatarAttachmentID *string
}

// Claims — проверенные данные access token.
type Claims struct {
	AccountID string
	SessionID string
}

// AccessToken — подписанный access token и момент его истечения.
type AccessToken struct {
	Value     string
	ExpiresAt time.Time
}

// Registration — полностью подготовленные записи первоначального сеанса.
// Пароль уже захеширован, refresh token представлен только хешем.
type Registration struct {
	AccountID        string
	Identifier       string
	DisplayName      string
	PasswordHash     string
	SessionID        string
	RefreshTokenHash string
	CreatedAt        time.Time
	SessionExpiresAt time.Time
}

// Rotation — параметры атомарной ротации refresh token.
type Rotation struct {
	TokenHash        string
	NewTokenHash     string
	Now              time.Time
	SessionExpiresAt time.Time
}

// RotationStatus — прикладной результат операции ротации.
type RotationStatus int

const (
	// RotationRejected — токен неизвестен, просрочен или сеанс неактивен.
	RotationRejected RotationStatus = iota
	// RotationSucceeded — пара выдана, сеанс продлён.
	RotationSucceeded
	// RotationReplay — предъявлен погашенный токен, сеанс отозван.
	RotationReplay
)

// RotationResult — результат ротации отдельно от инфраструктурной ошибки.
type RotationResult struct {
	Status    RotationStatus
	Profile   Profile
	SessionID string
	Access    AccessToken
}

// IssueAccess signs with the confirmed session claims inside the rotation transaction.
// It performs local work and must not publish the token before commit.
type IssueAccess func(context.Context, Claims) (AccessToken, error)

// RevokeRequest — параметры отзыва текущего или всех сеансов.
type RevokeRequest struct {
	AccountID string
	SessionID string
	All       bool
	Now       time.Time
}

// AccountRepository хранит аккаунты и читает профиль.
type AccountRepository interface {
	CreateWithSession(ctx context.Context, registration Registration) error
	Profile(ctx context.Context, accountID string) (Profile, error)
}

// SessionRepository меняет сеансы и refresh token в транзакции.
type SessionRepository interface {
	RotateRefresh(ctx context.Context, rotation Rotation, issue IssueAccess) (RotationResult, error)
	RevokeSessions(ctx context.Context, request RevokeRequest) error
}

// PasswordHasher хеширует и проверяет пароли.
type PasswordHasher interface {
	Hash(ctx context.Context, password string) (string, error)
	Verify(ctx context.Context, password, hash string) (bool, error)
}

// TokenCodec выдаёт и локально проверяет access JWT.
type TokenCodec interface {
	Issue(ctx context.Context, claims Claims) (AccessToken, error)
	Validate(ctx context.Context, token string) (Claims, error)
}

// RefreshTokens создаёт непрозрачный refresh token и его хеш.
type RefreshTokens interface {
	New() (token string, hash string, err error)
}

// Deps задаёт зависимости модуля Auth.
type Deps struct {
	Accounts      AccountRepository
	Sessions      SessionRepository
	Hasher        PasswordHasher
	Tokens        TokenCodec
	RefreshTokens RefreshTokens
	Clock         clock.Clock
	IDs           idgen.IDGenerator
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

// Readiness возвращает проверки готовности модуля. Проверка PostgreSQL
// подключается точкой сборки вместе с адаптером.
func (m *Module) Readiness() []httpserver.ReadinessCheck {
	return nil
}
