package postgres

import (
	"errors"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

// Проверка не требует БД: она фиксирует, какие ошибки драйвера адаптер считает
// «идентификатор занят». Реальные имена ограничений сверяет интеграционный тест.
func TestIsIdentifierConflict(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "unique violation on identifier",
			err:  &pgconn.PgError{Code: "23505", ConstraintName: "users_identifier_key"},
			want: true,
		},
		{
			name: "wrapped unique violation on identifier",
			err:  fmt.Errorf("insert user: %w", &pgconn.PgError{Code: "23505", ConstraintName: "users_identifier_key"}),
			want: true,
		},
		{
			name: "unique violation on primary key",
			err:  &pgconn.PgError{Code: "23505", ConstraintName: "users_pkey"},
			want: false,
		},
		{
			name: "check violation on identifier",
			err:  &pgconn.PgError{Code: "23514", ConstraintName: "users_identifier_check"},
			want: false,
		},
		{
			name: "same constraint name with another code",
			err:  &pgconn.PgError{Code: "23503", ConstraintName: "users_identifier_key"},
			want: false,
		},
		{
			name: "not a database error",
			err:  errors.New("connection reset"),
			want: false,
		},
		{
			name: "nil",
			err:  nil,
			want: false,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			if got := isIdentifierConflict(test.err); got != test.want {
				t.Errorf("isIdentifierConflict(%v) = %v, want %v", test.err, got, test.want)
			}
		})
	}
}
