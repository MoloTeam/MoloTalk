package auth

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

const (
	// PasswordMinLength — нижняя граница длины пароля по F_Pwd_1.
	PasswordMinLength = 6
	// PasswordMaxLength — верхняя граница длины пароля по F_Pwd_1.
	PasswordMaxLength = 64
	// IdentifierMaxLength — верхняя граница идентификатора из proto.
	IdentifierMaxLength = 254
	// DisplayNameMaxLength — верхняя граница имени из proto.
	DisplayNameMaxLength = 64
)

const allowedPasswordSpecial = `!@#$%^&*()_+-=[]{};:'"|,.<>/?`

// PasswordPolicyError перечисляет нарушенные правила F_Pwd_1–F_Pwd_3.
// Сообщение содержит только безопасное описание допустимых значений.
type PasswordPolicyError struct {
	Violations []string
}

func (e *PasswordPolicyError) Error() string {
	return "password policy: " + strings.Join(e.Violations, "; ")
}

// InvalidInputError сообщает о нарушении прикладного правила ввода.
type InvalidInputError struct {
	Message string
}

func (e *InvalidInputError) Error() string {
	return e.Message
}

// ErrIdentifierTaken сообщает о конфликте уникального идентификатора.
var ErrIdentifierTaken = errors.New("auth: identifier already taken")

// ErrUnauthenticated сообщает об отказе проверки идентичности.
var ErrUnauthenticated = errors.New("auth: unauthenticated")

// ErrProfileNotFound сообщает об отсутствии собственного профиля.
var ErrProfileNotFound = errors.New("auth: profile not found")

// ErrStorageUnavailable означает, что Auth не может обратиться к PostgreSQL.
var ErrStorageUnavailable = errors.New("auth: storage unavailable")

// NormalizeIdentifier канонизирует логин или e-mail для хранения.
// Хранение и уникальность работают с нижним регистром без крайних пробелов.
func NormalizeIdentifier(raw string) (string, error) {
	identifier := strings.ToLower(strings.TrimSpace(raw))
	if identifier == "" {
		return "", &InvalidInputError{Message: "identifier: must not be empty"}
	}
	if utf8.RuneCountInString(identifier) > IdentifierMaxLength {
		return "", &InvalidInputError{Message: "identifier: exceeds the allowed length"}
	}
	return identifier, nil
}

// ValidateDisplayName отклоняет имя из одних пробельных символов.
func ValidateDisplayName(raw string) error {
	if strings.TrimSpace(raw) == "" {
		return &InvalidInputError{Message: "display_name: must contain a non-space character"}
	}
	length := utf8.RuneCountInString(raw)
	if length < 1 || length > DisplayNameMaxLength {
		return &InvalidInputError{Message: "display_name: length must be between 1 and 64 characters"}
	}
	return nil
}

// CheckPassword возвращает нарушения F_Pwd_1–F_Pwd_3. Пустой результат
// означает, что пароль допустим.
func CheckPassword(password string) []string {
	violations := make([]string, 0, 3)
	length := utf8.RuneCountInString(password)

	if length < PasswordMinLength || length > PasswordMaxLength {
		violations = append(violations, fmt.Sprintf(
			"F_Pwd_1: length must be between %d and %d characters",
			PasswordMinLength, PasswordMaxLength))
	}

	if !onlyAllowedPasswordCharacters(password) {
		violations = append(violations, fmt.Sprintf(
			"F_Pwd_2: allowed characters are A-Z, a-z, 0-9 and %s",
			allowedPasswordSpecial))
	}

	var upper, lower, digit, special bool
	for _, character := range password {
		switch {
		case character >= 'A' && character <= 'Z':
			upper = true
		case character >= 'a' && character <= 'z':
			lower = true
		case character >= '0' && character <= '9':
			digit = true
		case strings.ContainsRune(allowedPasswordSpecial, character):
			special = true
		}
	}
	if !upper || !lower || !digit || !special {
		violations = append(violations, "F_Pwd_3: password must contain an uppercase letter, a lowercase letter, a digit and a special character")
	}

	return violations
}

func onlyAllowedPasswordCharacters(password string) bool {
	for _, character := range password {
		switch {
		case character >= 'A' && character <= 'Z':
		case character >= 'a' && character <= 'z':
		case character >= '0' && character <= '9':
		case character < utf8.RuneSelf && strings.ContainsRune(allowedPasswordSpecial, character):
		default:
			return false
		}
	}
	return true
}

// HashRefreshToken возвращает детерминированный SHA-256 непрозрачного
// refresh token. В PostgreSQL хранится только этот хеш.
func HashRefreshToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
