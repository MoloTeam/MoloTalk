// Package fixtures загружает детерминированные наборы данных в подготовленную
// схему. Загрузчик вызывается явно и никогда не запускается автоматически.
package fixtures

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// Set — имя набора данных.
type Set string

// Basic — базовый набор: пользователи, личный диалог и группа.
const Basic Set = "basic"

// ErrUnknownSet возвращается для набора, которого нет в реестре.
var ErrUnknownSet = errors.New("fixtures: unknown set")

// ErrNotEmpty возвращается, когда прикладные таблицы уже содержат записи.
var ErrNotEmpty = errors.New("fixtures: application tables are not empty")

const serviceTable = "goose_db_version"

// Load проверяет пустоту прикладных таблиц и записывает набор одной транзакцией.
func Load(ctx context.Context, db *sql.DB, set Set) error {
	statements, ok := sets[set]
	if !ok {
		return fmt.Errorf("%w: %s", ErrUnknownSet, set)
	}
	return LoadStatements(ctx, db, statements)
}

// LoadStatements записывает произвольный набор команд одной транзакцией после
// проверки пустоты прикладных таблиц. Проверки используют его, чтобы
// воспроизвести откат ошибочного набора.
func LoadStatements(ctx context.Context, db *sql.DB, statements []string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return errors.New("fixtures: begin transaction failed")
	}
	defer tx.Rollback()

	if err := ensureEmpty(ctx, tx); err != nil {
		return err
	}
	for _, statement := range statements {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return errors.New("fixtures: insert failed")
		}
	}

	if err := tx.Commit(); err != nil {
		return errors.New("fixtures: commit failed")
	}
	return nil
}

// ensureEmpty проверяет все прикладные таблицы, включая те, которые набор не
// заполняет: непустое состояние означает, что загрузка запрещена.
func ensureEmpty(ctx context.Context, tx *sql.Tx) error {
	rows, err := tx.QueryContext(ctx, `
		SELECT c.relname
		FROM pg_class c
		JOIN pg_namespace n ON n.oid = c.relnamespace AND n.nspname = 'public'
		WHERE c.relkind = 'r' AND c.relname <> '`+serviceTable+`'
		ORDER BY c.relname`)
	if err != nil {
		return errors.New("fixtures: list application tables failed")
	}

	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			rows.Close()
			return errors.New("fixtures: list application tables failed")
		}
		tables = append(tables, name)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return errors.New("fixtures: list application tables failed")
	}

	for _, table := range tables {
		query := fmt.Sprintf("SELECT count(*) FROM %s", pgx.Identifier{table}.Sanitize())
		var count int64
		if err := tx.QueryRowContext(ctx, query).Scan(&count); err != nil {
			return fmt.Errorf("fixtures: count %s failed", table)
		}
		if count > 0 {
			return fmt.Errorf("%w: %s", ErrNotEmpty, table)
		}
	}
	return nil
}
