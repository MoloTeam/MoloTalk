package auth_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/MoloTeam/MoloTalk/backend/internal/auth"
)

func TestPasswordPolicy(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		password   string
		violations []string
	}{
		{name: "six characters with all classes", password: "Aa1!aa"},
		{name: "sixty four characters with all classes", password: "Aa1!" + strings.Repeat("a", 60)},
		{name: "five characters", password: "Aa1!a", violations: []string{"F_Pwd_1"}},
		{name: "sixty five characters", password: "Aa1!" + strings.Repeat("a", 61), violations: []string{"F_Pwd_1"}},
		{name: "missing uppercase", password: "aa1!aaa", violations: []string{"F_Pwd_3"}},
		{name: "missing lowercase", password: "AA1!AAA", violations: []string{"F_Pwd_3"}},
		{name: "missing digit", password: "Aa!aaaa", violations: []string{"F_Pwd_3"}},
		{name: "missing special", password: "Aa1aaaa", violations: []string{"F_Pwd_3"}},
		{name: "space", password: "Aa1! aa", violations: []string{"F_Pwd_2"}},
		{name: "cyrillic", password: "Aa1!яя", violations: []string{"F_Pwd_2"}},
		{name: "emoji", password: "Aa1!🙂", violations: []string{"F_Pwd_2"}},
		{name: "control character", password: "Aa1!\x01a", violations: []string{"F_Pwd_2"}},
		{name: "backslash outside the allowed set", password: "Aa1!\\aa", violations: []string{"F_Pwd_2"}},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			violations := auth.CheckPassword(testCase.password)
			joined := strings.Join(violations, "; ")

			if len(testCase.violations) == 0 {
				if len(violations) != 0 {
					t.Fatalf("CheckPassword(%q) = %q, want no violations", testCase.password, joined)
				}
				return
			}
			for _, rule := range testCase.violations {
				if !strings.Contains(joined, rule) {
					t.Errorf("CheckPassword(%q) = %q, want rule %s", testCase.password, joined, rule)
				}
			}
			if strings.Contains(joined, testCase.password) {
				t.Errorf("policy error leaks the password: %q", joined)
			}
		})
	}
}

func TestPasswordPolicyReportsAllViolations(t *testing.T) {
	t.Parallel()

	// Короткий пароль без классов нарушает длину, состав и содержимое.
	violations := auth.CheckPassword("a b")
	joined := strings.Join(violations, "; ")
	for _, rule := range []string{"F_Pwd_1", "F_Pwd_2", "F_Pwd_3"} {
		if !strings.Contains(joined, rule) {
			t.Errorf("violations = %q, want %s", joined, rule)
		}
	}
}

func TestNormalizeIdentifier(t *testing.T) {
	t.Parallel()

	cases := []struct {
		raw      string
		expected string
	}{
		{raw: "Alice", expected: "alice"},
		{raw: " Bob@Example.COM ", expected: "bob@example.com"},
		{raw: "alice", expected: "alice"},
	}
	for _, testCase := range cases {
		got, err := auth.NormalizeIdentifier(testCase.raw)
		if err != nil {
			t.Fatalf("NormalizeIdentifier(%q): %v", testCase.raw, err)
		}
		if got != testCase.expected {
			t.Errorf("NormalizeIdentifier(%q) = %q, want %q", testCase.raw, got, testCase.expected)
		}
	}

	for _, raw := range []string{"", "   "} {
		if _, err := auth.NormalizeIdentifier(raw); err == nil {
			t.Errorf("NormalizeIdentifier(%q) succeeded, want invalid input", raw)
		}
	}
}

func TestValidateDisplayName(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"Alice", " Alice ", strings.Repeat("A", 64)} {
		if err := auth.ValidateDisplayName(name); err != nil {
			t.Errorf("ValidateDisplayName(%q): %v", name, err)
		}
	}
	for _, name := range []string{"", "   ", "\t\n", strings.Repeat("A", 65)} {
		if err := auth.ValidateDisplayName(name); err == nil {
			t.Errorf("ValidateDisplayName(%q) succeeded, want invalid input", name)
		}
	}
}

func TestInvalidInputErrorsAreSafe(t *testing.T) {
	t.Parallel()

	var invalid *auth.InvalidInputError
	_, err := auth.NormalizeIdentifier("   ")
	if !errors.As(err, &invalid) {
		t.Fatalf("error = %v, want InvalidInputError", err)
	}
	if strings.TrimSpace(invalid.Error()) == "" {
		t.Fatal("InvalidInputError has an empty message")
	}
}
