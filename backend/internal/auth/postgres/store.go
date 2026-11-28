// Package postgres реализует порты Auth поверх PostgreSQL и pgx.
package postgres

import (
	"context"
	"errors"
	"io"
	"net"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/MoloTeam/MoloTalk/backend/internal/auth"
)

// Store хранит аккаунты и сеансы Auth.
type Store struct {
	pool *pgxpool.Pool
}

// Open открывает пул и подтверждает доступность PostgreSQL. Ошибки не
// содержат DSN и текст драйвера с параметрами подключения.
func Open(ctx context.Context, dsn string) (*Store, error) {
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, errors.New("postgres: invalid database url")
	}
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, errors.New("postgres: database unavailable")
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, errors.New("postgres: database unavailable")
	}
	return &Store{pool: pool}, nil
}

// New создаёт хранилище на готовом пуле.
func New(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// Close освобождает пул.
func (s *Store) Close() {
	s.pool.Close()
}

// Check подтверждает готовность PostgreSQL для /readyz.
func (s *Store) Check(ctx context.Context) error {
	return s.pool.Ping(ctx)
}

// CreateWithSession атомарно сохраняет пользователя, учётные данные, сеанс и
// хеш refresh token. Коммит выполняется последним шагом.
func (s *Store) CreateWithSession(ctx context.Context, registration auth.Registration) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return databaseError(ctx, err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx,
		`INSERT INTO users (id, identifier, display_name, created_at) VALUES ($1, $2, $3, $4)`,
		registration.AccountID, registration.Identifier, registration.DisplayName, registration.CreatedAt,
	); err != nil {
		return identifierConflict(ctx, err)
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO user_auth (user_id, password_hash, password_changed_at) VALUES ($1, $2, $3)`,
		registration.AccountID, registration.PasswordHash, registration.CreatedAt,
	); err != nil {
		return databaseError(ctx, err)
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO sessions (id, user_id, created_at, expires_at) VALUES ($1, $2, $3, $4)`,
		registration.SessionID, registration.AccountID, registration.CreatedAt, registration.SessionExpiresAt,
	); err != nil {
		return databaseError(ctx, err)
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO refresh_tokens (token_hash, session_id, expires_at) VALUES ($1, $2, $3)`,
		registration.RefreshTokenHash, registration.SessionID, registration.SessionExpiresAt,
	); err != nil {
		return databaseError(ctx, err)
	}
	return databaseError(ctx, tx.Commit(ctx))
}

// Profile читает собственный профиль по идентификатору аккаунта.
func (s *Store) Profile(ctx context.Context, accountID string) (auth.Profile, error) {
	var profile auth.Profile
	var avatar pgtype.Text
	err := s.pool.QueryRow(ctx,
		`SELECT id, identifier, display_name, avatar_attachment_id FROM users WHERE id = $1`,
		accountID,
	).Scan(&profile.ID, &profile.Identifier, &profile.DisplayName, &avatar)
	if errors.Is(err, pgx.ErrNoRows) {
		return auth.Profile{}, auth.ErrProfileNotFound
	}
	if err != nil {
		return auth.Profile{}, databaseError(ctx, err)
	}
	if avatar.Valid {
		value := avatar.String
		profile.AvatarAttachmentID = &value
	}
	return profile, nil
}

// RotateRefresh сериализует изменения сеансов блокировкой строки users,
// перечитывает токен и сеанс и атомарно погашает токен, продлевает сеанс и
// сохраняет новый хеш. Погашенный токен действующего сеанса отзывает сеанс.
func (s *Store) RotateRefresh(ctx context.Context, rotation auth.Rotation, issue auth.IssueAccess) (auth.RotationResult, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return auth.RotationResult{}, databaseError(ctx, err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	accountID, err := tokenOwner(ctx, tx, rotation.TokenHash)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return auth.RotationResult{Status: auth.RotationRejected}, nil
		}
		return auth.RotationResult{}, databaseError(ctx, err)
	}
	if err := lockAccount(ctx, tx, accountID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return auth.RotationResult{Status: auth.RotationRejected}, nil
		}
		return auth.RotationResult{}, databaseError(ctx, err)
	}

	var (
		sessionID      string
		ownerID        string
		identifier     string
		displayName    string
		tokenExpires   time.Time
		sessionExpires time.Time
		consumed       pgtype.Timestamptz
		revoked        pgtype.Timestamptz
		avatar         pgtype.Text
	)
	err = tx.QueryRow(ctx, `
		SELECT rt.session_id, rt.expires_at, rt.consumed_at,
		       s.user_id, s.expires_at, s.revoked_at,
		       u.identifier, u.display_name, u.avatar_attachment_id
		FROM refresh_tokens rt
		JOIN sessions s ON s.id = rt.session_id
		JOIN users u ON u.id = s.user_id
		WHERE rt.token_hash = $1
		FOR UPDATE OF rt, s`, rotation.TokenHash,
	).Scan(&sessionID, &tokenExpires, &consumed,
		&ownerID, &sessionExpires, &revoked,
		&identifier, &displayName, &avatar)
	if errors.Is(err, pgx.ErrNoRows) {
		return auth.RotationResult{Status: auth.RotationRejected}, nil
	}
	if err != nil {
		return auth.RotationResult{}, databaseError(ctx, err)
	}

	if revoked.Valid || !sessionExpires.After(rotation.Now) {
		return auth.RotationResult{Status: auth.RotationRejected}, nil
	}

	if consumed.Valid {
		if _, err := tx.Exec(ctx,
			`UPDATE sessions SET revoked_at = $2 WHERE id = $1 AND revoked_at IS NULL`,
			sessionID, rotation.Now,
		); err != nil {
			return auth.RotationResult{}, databaseError(ctx, err)
		}
		if err := tx.Commit(ctx); err != nil {
			return auth.RotationResult{}, databaseError(ctx, err)
		}
		return auth.RotationResult{Status: auth.RotationReplay}, nil
	}

	if !tokenExpires.After(rotation.Now) {
		return auth.RotationResult{Status: auth.RotationRejected}, nil
	}

	access, err := issue(ctx, auth.Claims{AccountID: ownerID, SessionID: sessionID})
	if err != nil {
		return auth.RotationResult{}, err
	}

	if _, err := tx.Exec(ctx,
		`UPDATE refresh_tokens SET consumed_at = $2 WHERE token_hash = $1 AND consumed_at IS NULL`,
		rotation.TokenHash, rotation.Now,
	); err != nil {
		return auth.RotationResult{}, databaseError(ctx, err)
	}
	if _, err := tx.Exec(ctx,
		`UPDATE sessions SET expires_at = $2 WHERE id = $1`,
		sessionID, rotation.SessionExpiresAt,
	); err != nil {
		return auth.RotationResult{}, databaseError(ctx, err)
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO refresh_tokens (token_hash, session_id, expires_at) VALUES ($1, $2, $3)`,
		rotation.NewTokenHash, sessionID, rotation.SessionExpiresAt,
	); err != nil {
		return auth.RotationResult{}, databaseError(ctx, err)
	}
	if err := tx.Commit(ctx); err != nil {
		return auth.RotationResult{}, databaseError(ctx, err)
	}

	profile := auth.Profile{
		ID:          ownerID,
		Identifier:  identifier,
		DisplayName: displayName,
	}
	if avatar.Valid {
		value := avatar.String
		profile.AvatarAttachmentID = &value
	}

	return auth.RotationResult{
		Status:    auth.RotationSucceeded,
		Profile:   profile,
		SessionID: sessionID,
		Access:    access,
	}, nil
}

// RevokeSessions отзывает текущий сеанс или все сеансы пользователя.
// Блокировка users согласует отзыв с ротацией.
func (s *Store) RevokeSessions(ctx context.Context, request auth.RevokeRequest) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return databaseError(ctx, err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := lockAccount(ctx, tx, request.AccountID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return auth.ErrProfileNotFound
		}
		return databaseError(ctx, err)
	}

	if request.All {
		if _, err := tx.Exec(ctx,
			`UPDATE sessions SET revoked_at = $2 WHERE user_id = $1 AND revoked_at IS NULL`,
			request.AccountID, request.Now,
		); err != nil {
			return databaseError(ctx, err)
		}
	} else {
		if _, err := tx.Exec(ctx,
			`UPDATE sessions SET revoked_at = $3 WHERE id = $1 AND user_id = $2 AND revoked_at IS NULL`,
			request.SessionID, request.AccountID, request.Now,
		); err != nil {
			return databaseError(ctx, err)
		}
	}
	return databaseError(ctx, tx.Commit(ctx))
}

func tokenOwner(ctx context.Context, tx pgx.Tx, tokenHash string) (string, error) {
	var accountID string
	err := tx.QueryRow(ctx, `
		SELECT s.user_id
		FROM refresh_tokens rt
		JOIN sessions s ON s.id = rt.session_id
		WHERE rt.token_hash = $1`, tokenHash,
	).Scan(&accountID)
	return accountID, err
}

func lockAccount(ctx context.Context, tx pgx.Tx, accountID string) error {
	var locked string
	return tx.QueryRow(ctx, `SELECT id FROM users WHERE id = $1 FOR UPDATE`, accountID).Scan(&locked)
}

func identifierConflict(ctx context.Context, err error) error {
	var pgError *pgconn.PgError
	if errors.As(err, &pgError) && pgError.Code == "23505" && pgError.ConstraintName == "users_identifier_key" {
		return auth.ErrIdentifierTaken
	}
	return databaseError(ctx, err)
}

func databaseError(ctx context.Context, err error) error {
	if err == nil || ctx.Err() != nil {
		return err
	}

	var pgError *pgconn.PgError
	if errors.As(err, &pgError) {
		if strings.HasPrefix(pgError.Code, "08") || pgError.Code == "57P01" || pgError.Code == "57P02" || pgError.Code == "57P03" {
			return auth.ErrStorageUnavailable
		}
		return err
	}

	var connectError *pgconn.ConnectError
	var networkError *net.OpError
	if errors.As(err, &connectError) || errors.As(err, &networkError) || errors.Is(err, pgconn.ErrConnClosed) ||
		errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, net.ErrClosed) {
		return auth.ErrStorageUnavailable
	}
	return err
}
