package auth

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"unicode/utf8"
)

// Границы идентификатора и имени в символах; совпадают с CHECK-ограничениями
// таблицы users.
const (
	IdentifierMaxLength  = 254
	DisplayNameMaxLength = 64
)

// RegisterInput — данные регистрации. Пароль не выводится ни в строковом
// представлении, ни в slog: случайная передача структуры в журнал или в
// сообщение ошибки не раскрывает его.
type RegisterInput struct {
	Identifier  string
	DisplayName string
	Password    string
}

const redactedPassword = "[REDACTED]"

// String возвращает представление без пароля.
func (i RegisterInput) String() string {
	return fmt.Sprintf("RegisterInput{Identifier: %q, DisplayName: %q, Password: %s}",
		i.Identifier, i.DisplayName, redactedPassword)
}

// GoString возвращает представление без пароля для verb %#v.
func (i RegisterInput) GoString() string {
	return i.String()
}

// LogValue скрывает пароль при записи через log/slog.
func (i RegisterInput) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("identifier", i.Identifier),
		slog.String("display_name", i.DisplayName),
		slog.String("password", redactedPassword),
	)
}

// NormalizeIdentifier приводит идентификатор к виду, в котором он хранится и
// сравнивается: без крайних пробелов и в нижнем регистре. Так «Alice@Mail.com»
// и «alice@mail.com» — один идентификатор (F_Auth_3). Вход (Login) должен
// нормализовать идентификатор этой же функцией.
func NormalizeIdentifier(identifier string) string {
	return strings.ToLower(strings.TrimSpace(identifier))
}

// Register создаёт учётную запись (F_Auth_1).
//
// Порядок: нормализация и проверка идентификатора и имени, проверка пароля по
// политике (F_Auth_4), проверка уникальности идентификатора (F_Auth_3), хеш
// пароля с солью через PasswordHasher (NF_Sec_1), сохранение. Все проверки
// входных данных выполняются до обращения к хранилищу и до хеширования, так
// что отклонённая регистрация ничего не создаёт и не тратит ресурсы хешера.
//
// Ошибки: *ValidationError и *PasswordPolicyError — неверные входные данные,
// ErrIdentifierTaken — идентификатор занят, остальные — сбои зависимостей.
// Возвращаемый Account не содержит хеш пароля.
func (m *Module) Register(ctx context.Context, input RegisterInput) (Account, error) {
	identifier := NormalizeIdentifier(input.Identifier)
	if err := validateIdentifier(identifier); err != nil {
		return Account{}, err
	}
	displayName := strings.TrimSpace(input.DisplayName)
	if err := validateDisplayName(displayName); err != nil {
		return Account{}, err
	}
	if err := CheckPassword(input.Password); err != nil {
		return Account{}, err
	}

	_, err := m.deps.Accounts.FindByIdentifier(ctx, identifier)
	switch {
	case err == nil:
		return Account{}, ErrIdentifierTaken
	case !errors.Is(err, ErrAccountNotFound):
		return Account{}, fmt.Errorf("find account: %w", err)
	}

	hash, err := m.deps.Hasher.Hash(ctx, input.Password)
	if err != nil {
		return Account{}, fmt.Errorf("hash password: %w", err)
	}
	// Защита от неисправного хешера: пустое значение и пароль «как есть» в
	// хранилище попасть не должны.
	if hash == "" || hash == input.Password {
		return Account{}, errors.New("hash password: hasher returned an unusable hash")
	}

	id, err := m.deps.IDs.NewID()
	if err != nil {
		return Account{}, fmt.Errorf("generate account id: %w", err)
	}
	now := m.deps.Clock.Now().UTC()

	account := Account{
		ID:                id.String(),
		Identifier:        identifier,
		DisplayName:       displayName,
		PasswordHash:      hash,
		CreatedAt:         now,
		PasswordChangedAt: now,
	}
	if err := m.deps.Accounts.Create(ctx, account); err != nil {
		// Вторая регистрация с тем же идентификатором могла пройти проверку
		// выше одновременно; решает уникальность в хранилище.
		if errors.Is(err, ErrIdentifierTaken) {
			return Account{}, ErrIdentifierTaken
		}
		return Account{}, fmt.Errorf("create account: %w", err)
	}

	account.PasswordHash = ""
	return account, nil
}

func validateIdentifier(identifier string) error {
	if identifier == "" {
		return &ValidationError{Field: FieldIdentifier, Message: "Логин (e-mail) не может быть пустым"}
	}
	if utf8.RuneCountInString(identifier) > IdentifierMaxLength {
		return &ValidationError{
			Field:   FieldIdentifier,
			Message: fmt.Sprintf("Логин (e-mail) не должен быть длиннее %d символов", IdentifierMaxLength),
		}
	}
	return nil
}

func validateDisplayName(displayName string) error {
	if displayName == "" {
		return &ValidationError{Field: FieldDisplayName, Message: "Имя не может быть пустым"}
	}
	if utf8.RuneCountInString(displayName) > DisplayNameMaxLength {
		return &ValidationError{
			Field:   FieldDisplayName,
			Message: fmt.Sprintf("Имя не должно быть длиннее %d символов", DisplayNameMaxLength),
		}
	}
	return nil
}
