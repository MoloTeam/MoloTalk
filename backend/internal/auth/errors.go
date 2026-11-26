package auth

import "errors"

var (
	// ErrAccountNotFound возвращает AccountRepository, когда учётной записи с
	// таким идентификатором нет.
	ErrAccountNotFound = errors.New("auth: account not found")

	// ErrIdentifierTaken означает, что идентификатор уже используется другой
	// учётной записью (F_Auth_3).
	ErrIdentifierTaken = errors.New("auth: identifier is already taken")
)

// Имена полей запроса, к которым относятся ошибки проверки.
const (
	FieldIdentifier  = "identifier"
	FieldDisplayName = "display_name"
	FieldPassword    = "password"
)

// ValidationError — нарушение входных данных. Message предназначен для
// показа пользователю и не содержит значений полей.
type ValidationError struct {
	Field   string
	Message string
}

// Error возвращает сообщение для пользователя.
func (e *ValidationError) Error() string {
	return e.Message
}
