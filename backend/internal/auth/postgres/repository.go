// Package postgres реализует порт auth.AccountRepository на PostgreSQL.
// Адаптер работает с таблицами users и user_auth из миграции 00001 и не
// содержит правил предметной области: нормализация идентификатора и проверка
// пароля выполняются в пакете auth до обращения к хранилищу.
package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/MoloTeam/MoloTalk/backend/internal/auth"
)

const (
	// uniqueViolation — SQLSTATE нарушения UNIQUE.
	uniqueViolation = "23505"
	// identifierConstraint — имя UNIQUE-ограничения users.identifier,
	// которое PostgreSQL выдаёт колонке по умолчанию.
	identifierConstraint = "users_identifier_key"
)

// Идентификаторы передаются как char(26): так параметры совпадают с типом
// базового домена ulid, а сравнение использует индекс первичного ключа.
const (
	findByIdentifierSQL = `
SELECT u.id::text, u.identifier, u.display_name, u.created_at, a.password_hash, a.password_changed_at
FROM users u
LEFT JOIN user_auth a ON a.user_id = u.id
WHERE u.identifier = $1`

	insertUserSQL = `
INSERT INTO users (id, identifier, display_name, created_at)
VALUES ($1::char(26), $2, $3, $4)`

	insertCredentialsSQL = `
INSERT INTO user_auth (user_id, password_hash, password_changed_at)
VALUES ($1::char(26), $2, $3)`

	updateCredentialsSQL = `
UPDATE user_auth
SET password_hash = $2, password_changed_at = $3
WHERE user_id = $1::char(26)`
)

// Repository хранит учётные записи в PostgreSQL.
type Repository struct {
	db *sql.DB
}

var _ auth.AccountRepository = (*Repository)(nil)

// New создаёт репозиторий поверх пула соединений.
func New(db *sql.DB) *Repository {
	return &Repository{db: db}
}

// FindByIdentifier возвращает учётную запись по нормализованному
// идентификатору или auth.ErrAccountNotFound. Запись пользователя без
// учётных данных (например, из набора фикстур) тоже считается найденной:
// у неё пустой PasswordHash, поэтому идентификатор остаётся занятым.
func (r *Repository) FindByIdentifier(ctx context.Context, identifier string) (auth.Account, error) {
	var account auth.Account
	var passwordHash sql.NullString
	var changedAt sql.NullTime
	err := r.db.QueryRowContext(ctx, findByIdentifierSQL, identifier).Scan(
		&account.ID,
		&account.Identifier,
		&account.DisplayName,
		&account.CreatedAt,
		&passwordHash,
		&changedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return auth.Account{}, auth.ErrAccountNotFound
	}
	if err != nil {
		return auth.Account{}, fmt.Errorf("find account: %w", err)
	}

	account.PasswordHash = passwordHash.String
	account.PasswordChangedAt = changedAt.Time
	return account, nil
}

// Create записывает пользователя и его учётные данные одной транзакцией.
// Нарушение уникальности идентификатора, в том числе при гонке двух
// регистраций, возвращается как auth.ErrIdentifierTaken. Остальные нарушения
// схемы (CHECK, FK, повтор ID) возвращаются как обычные ошибки хранилища.
func (r *Repository) Create(ctx context.Context, account auth.Account) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, insertUserSQL,
		account.ID, account.Identifier, account.DisplayName, account.CreatedAt,
	); err != nil {
		if isIdentifierConflict(err) {
			return auth.ErrIdentifierTaken
		}
		return fmt.Errorf("insert user: %w", err)
	}

	if _, err := tx.ExecContext(ctx, insertCredentialsSQL,
		account.ID, account.PasswordHash, account.PasswordChangedAt,
	); err != nil {
		return fmt.Errorf("insert credentials: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit account: %w", err)
	}
	return nil
}

// Update сохраняет изменяемые учётные данные: хеш пароля и время его смены.
// Профиль пользователя (имя, аватар) этим методом не меняется. Для
// неизвестного ID возвращается auth.ErrAccountNotFound.
func (r *Repository) Update(ctx context.Context, account auth.Account) error {
	result, err := r.db.ExecContext(ctx, updateCredentialsSQL,
		account.ID, account.PasswordHash, account.PasswordChangedAt)
	if err != nil {
		return fmt.Errorf("update credentials: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("update credentials: %w", err)
	}
	if affected == 0 {
		return auth.ErrAccountNotFound
	}
	return nil
}

// isIdentifierConflict сообщает, что ошибка — нарушение UNIQUE именно по
// идентификатору, а не по первичному ключу или другому ограничению.
func isIdentifierConflict(err error) bool {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return false
	}
	return pgErr.Code == uniqueViolation && pgErr.ConstraintName == identifierConstraint
}
