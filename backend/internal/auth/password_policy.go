package auth

import (
	"fmt"
	"strings"
)

// Границы длины пароля в символах (F_Pwd_1).
const (
	PasswordMinLength = 6
	PasswordMaxLength = 64
)

// PasswordSpecialCharacters — допустимые специальные символы (F_Pwd_2).
const PasswordSpecialCharacters = `!@#$%^&*()_+-=[]{};:'"|,.<>/?`

// PasswordRule — идентификатор правила политики пароля.
type PasswordRule string

// Правила политики пароля.
const (
	// PasswordRuleLength — длина от PasswordMinLength до PasswordMaxLength (F_Pwd_1).
	PasswordRuleLength PasswordRule = "length"
	// PasswordRuleCharacters — только разрешённые символы (F_Pwd_2).
	PasswordRuleCharacters PasswordRule = "characters"
	// PasswordRuleComposition — обязательные классы символов (F_Pwd_3).
	PasswordRuleComposition PasswordRule = "composition"
)

// PasswordViolation — одно нарушенное правило. Message предназначен для
// показа пользователю: он описывает требование, но не содержит пароль.
type PasswordViolation struct {
	Rule    PasswordRule
	Message string
}

// ValidatePassword проверяет пароль по политике F_Pwd_1–F_Pwd_3 и возвращает
// все нарушенные правила в порядке length, characters, composition. Пустой
// результат означает, что пароль допустим.
//
// Правило не зависит от БД и транспорта, поэтому его используют и регистрация,
// и смена пароля. Длина считается в символах (рунах), а не в байтах. Классы
// символов определяются только по ASCII: кириллическая заглавная буква не
// считается «заглавной буквой» политики.
func ValidatePassword(password string) []PasswordViolation {
	length := 0
	hasUpper := false
	hasLower := false
	hasDigit := false
	hasSpecial := false
	hasForbidden := false
	// Невалидные байты UTF-8 range превращает в U+FFFD, а он не входит в
	// разрешённый набор, поэтому они попадают в hasForbidden.
	for _, r := range password {
		length++
		switch {
		case r >= 'A' && r <= 'Z':
			hasUpper = true
		case r >= 'a' && r <= 'z':
			hasLower = true
		case r >= '0' && r <= '9':
			hasDigit = true
		case strings.ContainsRune(PasswordSpecialCharacters, r):
			hasSpecial = true
		default:
			hasForbidden = true
		}
	}

	var violations []PasswordViolation
	if length < PasswordMinLength || length > PasswordMaxLength {
		violations = append(violations, PasswordViolation{Rule: PasswordRuleLength, Message: lengthMessage()})
	}
	if hasForbidden {
		violations = append(violations, PasswordViolation{Rule: PasswordRuleCharacters, Message: charactersMessage()})
	}
	if !hasUpper || !hasLower || !hasDigit || !hasSpecial {
		violations = append(violations, PasswordViolation{Rule: PasswordRuleComposition, Message: compositionMessage()})
	}
	return violations
}

func lengthMessage() string {
	return fmt.Sprintf("Длина пароля должна быть от %d до %d символов", PasswordMinLength, PasswordMaxLength)
}

func charactersMessage() string {
	return "Пароль может содержать только латинские буквы A–Z, a–z, цифры 0–9 и специальные символы " +
		spacedSpecialCharacters() +
		"; пробелы, кириллица, эмодзи, управляющие и прочие символы не допускаются"
}

func compositionMessage() string {
	return "Пароль должен содержать не менее одной заглавной латинской буквы, " +
		"одной строчной латинской буквы, одной цифры и одного специального символа"
}

// PasswordPolicyError — пароль нарушает политику. Сообщения не содержат пароль.
type PasswordPolicyError struct {
	Violations []PasswordViolation
}

// CheckPassword возвращает *PasswordPolicyError, если пароль нарушает
// политику, и nil, если пароль допустим.
func CheckPassword(password string) error {
	violations := ValidatePassword(password)
	if len(violations) == 0 {
		return nil
	}
	return &PasswordPolicyError{Violations: violations}
}

// Error объединяет сообщения всех нарушенных правил.
func (e *PasswordPolicyError) Error() string {
	messages := make([]string, 0, len(e.Violations))
	for _, violation := range e.Violations {
		messages = append(messages, violation.Message)
	}
	return strings.Join(messages, "; ")
}

// Rules возвращает идентификаторы нарушенных правил.
func (e *PasswordPolicyError) Rules() []PasswordRule {
	rules := make([]PasswordRule, 0, len(e.Violations))
	for _, violation := range e.Violations {
		rules = append(rules, violation.Rule)
	}
	return rules
}

func spacedSpecialCharacters() string {
	return strings.Join(strings.Split(PasswordSpecialCharacters, ""), " ")
}
