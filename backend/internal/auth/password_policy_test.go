package auth_test

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/MoloTeam/MoloTalk/backend/internal/auth"
)

// validBase — допустимый пароль из 6 символов: все классы присутствуют.
const validBase = "Aa1!xx"

// padded возвращает допустимый по составу пароль длиной ровно n символов (n >= 4).
func padded(n int) string {
	return "Aa1!" + strings.Repeat("x", n-4)
}

func ruleSet(violations []auth.PasswordViolation) []auth.PasswordRule {
	rules := make([]auth.PasswordRule, 0, len(violations))
	for _, violation := range violations {
		rules = append(rules, violation.Rule)
	}
	return rules
}

func TestValidatePasswordLength(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		password string
		want     []auth.PasswordRule
	}{
		{name: "empty", password: "", want: []auth.PasswordRule{auth.PasswordRuleLength, auth.PasswordRuleComposition}},
		{name: "5 characters", password: padded(5), want: []auth.PasswordRule{auth.PasswordRuleLength}},
		{name: "6 characters", password: padded(6), want: nil},
		{name: "7 characters", password: padded(7), want: nil},
		{name: "32 characters", password: padded(32), want: nil},
		{name: "63 characters", password: padded(63), want: nil},
		{name: "64 characters", password: padded(64), want: nil},
		{name: "65 characters", password: padded(65), want: []auth.PasswordRule{auth.PasswordRuleLength}},
		{name: "1000 characters", password: padded(1000), want: []auth.PasswordRule{auth.PasswordRuleLength}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got := ruleSet(auth.ValidatePassword(test.password))
			if !slices.Equal(got, test.want) {
				t.Errorf("rules = %v, want %v", got, test.want)
			}
		})
	}
}

func TestValidatePasswordLengthCountsCharactersNotBytes(t *testing.T) {
	t.Parallel()

	// 64 символа допустимы, хотя в байтах строка длиннее: недопустимые руны
	// дают только нарушение набора символов, но не длины.
	multibyte := "Aa1!" + strings.Repeat("ж", 60)
	got := ruleSet(auth.ValidatePassword(multibyte))
	want := []auth.PasswordRule{auth.PasswordRuleCharacters}
	if !slices.Equal(got, want) {
		t.Errorf("64 runes: rules = %v, want %v", got, want)
	}

	// Эмодзи — один символ: 5 рун — это короткий пароль, а не 8 байт.
	got = ruleSet(auth.ValidatePassword("Aa1!😀"))
	want = []auth.PasswordRule{auth.PasswordRuleLength, auth.PasswordRuleCharacters}
	if !slices.Equal(got, want) {
		t.Errorf("5 runes with emoji: rules = %v, want %v", got, want)
	}
}

func TestValidatePasswordRejectsForbiddenCharacters(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		character string
	}{
		{name: "space", character: " "},
		{name: "tab", character: "\t"},
		{name: "newline", character: "\n"},
		{name: "NUL control", character: "\x00"},
		{name: "DEL control", character: "\x7f"},
		{name: "no-break space", character: "\u00a0"},
		{name: "zero-width space", character: "\u200b"},
		{name: "cyrillic lower", character: "ж"},
		{name: "cyrillic upper", character: "Ж"},
		{name: "emoji", character: "😀"},
		{name: "latin with diacritic", character: "é"},
		{name: "copyright sign", character: "©"},
		{name: "full-width latin", character: "Ａ"},
		{name: "tilde", character: "~"},
		{name: "backtick", character: "`"},
		{name: "backslash", character: `\`},
		{name: "invalid utf-8", character: "\xff"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			password := "Aa1!x" + test.character + "x"
			got := ruleSet(auth.ValidatePassword(password))
			want := []auth.PasswordRule{auth.PasswordRuleCharacters}
			if !slices.Equal(got, want) {
				t.Errorf("rules = %v, want %v", got, want)
			}
		})
	}
}

func TestValidatePasswordAcceptsEverySpecialCharacter(t *testing.T) {
	t.Parallel()

	if got := len(auth.PasswordSpecialCharacters); got != 29 {
		t.Fatalf("special characters = %d, want the 29 listed in F_Pwd_2", got)
	}
	for _, special := range auth.PasswordSpecialCharacters {
		password := "Aa1" + string(special) + "xx"
		if violations := auth.ValidatePassword(password); len(violations) != 0 {
			t.Errorf("special %q rejected: %v", special, ruleSet(violations))
		}
	}
}

// TestValidatePasswordAsciiClassification независимо от реализации перебирает
// все 128 ASCII-символов: допустимы буквы, цифры и ровно перечисленные в
// F_Pwd_2 специальные символы.
func TestValidatePasswordAsciiClassification(t *testing.T) {
	t.Parallel()

	const listed = "!@#$%^&*()_+-=[]{};:'\"|,.<>/?"
	for code := 0; code < 128; code++ {
		character := rune(code)
		allowed := (character >= 'A' && character <= 'Z') ||
			(character >= 'a' && character <= 'z') ||
			(character >= '0' && character <= '9') ||
			strings.ContainsRune(listed, character)

		violations := auth.ValidatePassword(validBase + string(character))
		if allowed && len(violations) != 0 {
			t.Errorf("%q (0x%02x) rejected: %v", character, code, ruleSet(violations))
		}
		if !allowed && !slices.Equal(ruleSet(violations), []auth.PasswordRule{auth.PasswordRuleCharacters}) {
			t.Errorf("%q (0x%02x) violations = %v, want [characters]", character, code, ruleSet(violations))
		}
	}
}

func TestValidatePasswordRequiresEveryCharacterClass(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		password string
	}{
		{name: "no uppercase", password: "abc1!x"},
		{name: "no lowercase", password: "ABC1!X"},
		{name: "no digit", password: "Abcd!x"},
		{name: "no special", password: "Abcd1x"},
		{name: "only letters", password: "Abcdef"},
		{name: "only digits", password: "123456"},
		{name: "only specials", password: "!@#$%^"},
		{name: "cyrillic uppercase is not an uppercase letter", password: "Жbc1!x"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got := ruleSet(auth.ValidatePassword(test.password))
			if !slices.Contains(got, auth.PasswordRuleComposition) {
				t.Errorf("rules = %v, want to contain composition", got)
			}
		})
	}
}

func TestValidatePasswordReportsEveryViolatedRule(t *testing.T) {
	t.Parallel()

	got := ruleSet(auth.ValidatePassword("ж"))
	want := []auth.PasswordRule{auth.PasswordRuleLength, auth.PasswordRuleCharacters, auth.PasswordRuleComposition}
	if !slices.Equal(got, want) {
		t.Errorf("rules = %v, want %v", got, want)
	}
}

func TestPasswordViolationMessagesDescribeRequirements(t *testing.T) {
	t.Parallel()

	messages := map[auth.PasswordRule]string{}
	for _, violation := range auth.ValidatePassword("ж") {
		messages[violation.Rule] = violation.Message
	}

	for _, fragment := range []string{"6", "64"} {
		if !strings.Contains(messages[auth.PasswordRuleLength], fragment) {
			t.Errorf("length message %q does not mention %q", messages[auth.PasswordRuleLength], fragment)
		}
	}

	characters := messages[auth.PasswordRuleCharacters]
	for _, fragment := range []string{"A–Z", "a–z", "0–9"} {
		if !strings.Contains(characters, fragment) {
			t.Errorf("characters message %q does not mention %q", characters, fragment)
		}
	}
	for _, special := range auth.PasswordSpecialCharacters {
		if !strings.ContainsRune(characters, special) {
			t.Errorf("characters message does not list %q", special)
		}
	}

	composition := messages[auth.PasswordRuleComposition]
	for _, fragment := range []string{"заглавной", "строчной", "цифры", "специального"} {
		if !strings.Contains(composition, fragment) {
			t.Errorf("composition message %q does not mention %q", composition, fragment)
		}
	}
}

func TestCheckPassword(t *testing.T) {
	t.Parallel()

	if err := auth.CheckPassword(validBase); err != nil {
		t.Fatalf("valid password: err = %v, want nil", err)
	}

	err := auth.CheckPassword("short")
	var policy *auth.PasswordPolicyError
	if !errors.As(err, &policy) {
		t.Fatalf("err = %v, want *PasswordPolicyError", err)
	}
	if !slices.Contains(policy.Rules(), auth.PasswordRuleLength) {
		t.Errorf("rules = %v, want to contain length", policy.Rules())
	}
}

func TestPasswordPolicyErrorDoesNotLeakPassword(t *testing.T) {
	t.Parallel()

	passwords := []string{"Zq9!Секрет", "Zq9 !uniquetoken", "short7Q", padded(70) + "uniquetoken"}
	for _, password := range passwords {
		err := auth.CheckPassword(password)
		if err == nil {
			continue
		}
		if strings.Contains(err.Error(), password) {
			t.Errorf("error text contains the password %q", password)
		}
	}
}
