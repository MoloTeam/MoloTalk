package migrations

import (
	"context"
	"database/sql"
)

// Schema — нормализованный снимок прикладной схемы. Служебные объекты Goose в
// снимок не входят.
type Schema struct {
	Domain      *Domain      `json:"domain"`
	Tables      []Table      `json:"tables"`
	Constraints []Constraint `json:"constraints"`
	Indexes     []Index      `json:"indexes"`
}

// Domain — домен схемы и его ограничение.
type Domain struct {
	Name     string `json:"name"`
	BaseType string `json:"base_type"`
	Check    string `json:"check"`
}

// Table — таблица и её столбцы в порядке объявления.
type Table struct {
	Name    string   `json:"name"`
	Columns []Column `json:"columns"`
}

// Column — столбец таблицы.
type Column struct {
	Name    string `json:"name"`
	Type    string `json:"type"`
	NotNull bool   `json:"not_null"`
	Default string `json:"default"`
}

// Constraint — ограничение таблицы: p, u, f или c.
type Constraint struct {
	Table      string `json:"table"`
	Name       string `json:"name"`
	Type       string `json:"type"`
	Definition string `json:"definition"`
}

// Index — явный индекс, не обслуживающий ограничение.
type Index struct {
	Table      string `json:"table"`
	Name       string `json:"name"`
	Definition string `json:"definition"`
}

const serviceTable = "goose_db_version"

// Snapshot читает прикладную схему из pg_catalog в стабильном порядке.
func Snapshot(ctx context.Context, db *sql.DB) (Schema, error) {
	schema := Schema{Domain: nil, Tables: []Table{}, Constraints: []Constraint{}, Indexes: []Index{}}

	domain, err := readDomain(ctx, db)
	if err != nil {
		return Schema{}, err
	}
	schema.Domain = domain

	tables, err := readTables(ctx, db)
	if err != nil {
		return Schema{}, err
	}
	schema.Tables = tables

	constraints, err := readConstraints(ctx, db)
	if err != nil {
		return Schema{}, err
	}
	schema.Constraints = constraints

	indexes, err := readIndexes(ctx, db)
	if err != nil {
		return Schema{}, err
	}
	schema.Indexes = indexes

	return schema, nil
}

func readDomain(ctx context.Context, db *sql.DB) (*Domain, error) {
	row := db.QueryRowContext(ctx, `
		SELECT t.typname,
		       format_type(t.typbasetype, t.typtypmod),
		       COALESCE((
		           SELECT pg_get_constraintdef(c.oid)
		           FROM pg_constraint c
		           WHERE c.contypid = t.oid
		           ORDER BY c.conname
		           LIMIT 1
		       ), '')
		FROM pg_type t
		JOIN pg_namespace n ON n.oid = t.typnamespace
		WHERE n.nspname = 'public' AND t.typtype = 'd'
		ORDER BY t.typname
		LIMIT 1`)

	var domain Domain
	switch err := row.Scan(&domain.Name, &domain.BaseType, &domain.Check); {
	case err == sql.ErrNoRows:
		return nil, nil
	case err != nil:
		return nil, err
	default:
		return &domain, nil
	}
}

func readTables(ctx context.Context, db *sql.DB) ([]Table, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT c.relname, a.attname,
		       format_type(a.atttypid, a.atttypmod),
		       a.attnotnull,
		       COALESCE(pg_get_expr(d.adbin, d.adrelid), '')
		FROM pg_class c
		JOIN pg_namespace n ON n.oid = c.relnamespace AND n.nspname = 'public'
		JOIN pg_attribute a ON a.attrelid = c.oid AND a.attnum > 0 AND NOT a.attisdropped
		LEFT JOIN pg_attrdef d ON d.adrelid = c.oid AND d.adnum = a.attnum
		WHERE c.relkind = 'r' AND c.relname <> '`+serviceTable+`'
		ORDER BY c.relname, a.attnum`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	tables := []Table{}
	for rows.Next() {
		var table, column, columnType, defaultExpression string
		var notNull bool
		if err := rows.Scan(&table, &column, &columnType, &notNull, &defaultExpression); err != nil {
			return nil, err
		}
		if len(tables) == 0 || tables[len(tables)-1].Name != table {
			tables = append(tables, Table{Name: table})
		}
		index := len(tables) - 1
		tables[index].Columns = append(tables[index].Columns, Column{
			Name:    column,
			Type:    columnType,
			NotNull: notNull,
			Default: defaultExpression,
		})
	}
	return tables, rows.Err()
}

func readConstraints(ctx context.Context, db *sql.DB) ([]Constraint, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT c.relname, con.conname, con.contype::text, pg_get_constraintdef(con.oid)
		FROM pg_constraint con
		JOIN pg_class c ON c.oid = con.conrelid
		JOIN pg_namespace n ON n.oid = c.relnamespace AND n.nspname = 'public'
		WHERE c.relname <> '`+serviceTable+`'
		ORDER BY c.relname, con.conname`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	constraints := []Constraint{}
	for rows.Next() {
		var constraint Constraint
		if err := rows.Scan(&constraint.Table, &constraint.Name, &constraint.Type, &constraint.Definition); err != nil {
			return nil, err
		}
		constraints = append(constraints, constraint)
	}
	return constraints, rows.Err()
}

func readIndexes(ctx context.Context, db *sql.DB) ([]Index, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT c.relname, i.relname, pg_get_indexdef(x.indexrelid)
		FROM pg_index x
		JOIN pg_class c ON c.oid = x.indrelid
		JOIN pg_class i ON i.oid = x.indexrelid
		JOIN pg_namespace n ON n.oid = c.relnamespace AND n.nspname = 'public'
		WHERE NOT x.indisprimary
		  AND NOT EXISTS (SELECT 1 FROM pg_constraint con WHERE con.conindid = x.indexrelid)
		  AND c.relname <> '`+serviceTable+`'
		ORDER BY c.relname, i.relname`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	indexes := []Index{}
	for rows.Next() {
		var index Index
		if err := rows.Scan(&index.Table, &index.Name, &index.Definition); err != nil {
			return nil, err
		}
		indexes = append(indexes, index)
	}
	return indexes, rows.Err()
}
