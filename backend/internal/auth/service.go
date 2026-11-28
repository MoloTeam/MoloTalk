package auth

import (
	"context"
	"errors"
	"strings"
	"time"
)

// RegisterInput — входные данные регистрации до нормализации.
type RegisterInput struct {
	Identifier  string
	DisplayName string
	Password    string
}

// TokenPair — выданная пара токенов и собственный профиль.
type TokenPair struct {
	Profile          Profile
	SessionID        string
	AccessToken      string
	RefreshToken     string
	AccessExpiresAt  time.Time
	RefreshExpiresAt time.Time
}

// Register создаёт аккаунт, учётные данные, сеанс и refresh-хеш одной
// транзакцией и возвращает пару после commit.
func (m *Module) Register(ctx context.Context, input RegisterInput) (TokenPair, error) {
	identifier, err := NormalizeIdentifier(input.Identifier)
	if err != nil {
		return TokenPair{}, err
	}
	if err := ValidateDisplayName(input.DisplayName); err != nil {
		return TokenPair{}, err
	}
	if violations := CheckPassword(input.Password); len(violations) > 0 {
		return TokenPair{}, &PasswordPolicyError{Violations: violations}
	}

	passwordHash, err := m.deps.Hasher.Hash(ctx, input.Password)
	if err != nil {
		return TokenPair{}, err
	}

	accountID, err := m.deps.IDs.NewID()
	if err != nil {
		return TokenPair{}, err
	}
	sessionID, err := m.deps.IDs.NewID()
	if err != nil {
		return TokenPair{}, err
	}

	refreshToken, refreshHash, err := m.deps.RefreshTokens.New()
	if err != nil {
		return TokenPair{}, err
	}

	now := m.deps.Clock.Now().UTC().Truncate(time.Second)
	expiresAt := now.Add(SessionTTL)
	access, err := m.deps.Tokens.Issue(ctx, Claims{
		AccountID: accountID.String(),
		SessionID: sessionID.String(),
	})
	if err != nil {
		return TokenPair{}, err
	}

	registration := Registration{
		AccountID:        accountID.String(),
		Identifier:       identifier,
		DisplayName:      input.DisplayName,
		PasswordHash:     passwordHash,
		SessionID:        sessionID.String(),
		RefreshTokenHash: refreshHash,
		CreatedAt:        now,
		SessionExpiresAt: expiresAt,
	}
	if err := m.deps.Accounts.CreateWithSession(ctx, registration); err != nil {
		if errors.Is(err, ErrIdentifierTaken) {
			return TokenPair{}, ErrIdentifierTaken
		}
		return TokenPair{}, err
	}

	return TokenPair{
		Profile: Profile{
			ID:          registration.AccountID,
			Identifier:  registration.Identifier,
			DisplayName: registration.DisplayName,
		},
		SessionID:        registration.SessionID,
		AccessToken:      access.Value,
		RefreshToken:     refreshToken,
		AccessExpiresAt:  access.ExpiresAt,
		RefreshExpiresAt: expiresAt,
	}, nil
}

// Refresh атомарно погашает refresh token, продлевает тот же сеанс и выдаёт
// новую пару. Погашенный токен действующего сеанса отзывает сеанс.
func (m *Module) Refresh(ctx context.Context, refreshToken string) (TokenPair, error) {
	if strings.TrimSpace(refreshToken) == "" {
		return TokenPair{}, ErrUnauthenticated
	}

	newToken, newHash, err := m.deps.RefreshTokens.New()
	if err != nil {
		return TokenPair{}, err
	}

	now := m.deps.Clock.Now().UTC().Truncate(time.Second)
	expiresAt := now.Add(SessionTTL)
	result, err := m.deps.Sessions.RotateRefresh(ctx, Rotation{
		TokenHash:        HashRefreshToken(refreshToken),
		NewTokenHash:     newHash,
		Now:              now,
		SessionExpiresAt: expiresAt,
	}, m.deps.Tokens.Issue)
	if err != nil {
		return TokenPair{}, err
	}

	if result.Status != RotationSucceeded {
		return TokenPair{}, ErrUnauthenticated
	}

	return TokenPair{
		Profile:          result.Profile,
		SessionID:        result.SessionID,
		AccessToken:      result.Access.Value,
		RefreshToken:     newToken,
		AccessExpiresAt:  result.Access.ExpiresAt,
		RefreshExpiresAt: expiresAt,
	}, nil
}

// GetMe читает собственный профиль по подтверждённому sub.
func (m *Module) GetMe(ctx context.Context, claims Claims) (Profile, error) {
	profile, err := m.deps.Accounts.Profile(ctx, claims.AccountID)
	if err != nil {
		if errors.Is(err, ErrProfileNotFound) {
			return Profile{}, ErrUnauthenticated
		}
		return Profile{}, err
	}
	return profile, nil
}

// Logout отзывает текущий сеанс или все сеансы пользователя.
func (m *Module) Logout(ctx context.Context, claims Claims, allSessions bool) error {
	err := m.deps.Sessions.RevokeSessions(ctx, RevokeRequest{
		AccountID: claims.AccountID,
		SessionID: claims.SessionID,
		All:       allSessions,
		Now:       m.deps.Clock.Now().UTC().Truncate(time.Second),
	})
	if errors.Is(err, ErrProfileNotFound) {
		return ErrUnauthenticated
	}
	return err
}

// Authenticate локально проверяет access JWT и возвращает подтверждённые
// claims. Отзыв сеанса не делает уже выданный access недействительным: он
// действует до своего exp.
func (m *Module) Authenticate(ctx context.Context, raw string) (Claims, error) {
	token := strings.TrimSpace(raw)
	if token == "" {
		return Claims{}, ErrUnauthenticated
	}
	claims, err := m.deps.Tokens.Validate(ctx, token)
	if err != nil || claims.AccountID == "" || claims.SessionID == "" {
		return Claims{}, ErrUnauthenticated
	}
	return claims, nil
}
