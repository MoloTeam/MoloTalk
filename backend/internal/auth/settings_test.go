package auth_test

import (
	"strings"
	"testing"

	"github.com/MoloTeam/MoloTalk/backend/internal/auth"
)

func TestLoadSettings(t *testing.T) {
	t.Parallel()

	values := map[string]string{
		auth.DatabaseURLVariable:    " postgres://user:secret@db/molotalk ",
		auth.SigningKeyFileVariable: " /run/secrets/auth-jwt-signing-key ",
	}
	settings, err := auth.LoadSettings(func(key string) string { return values[key] })
	if err != nil {
		t.Fatalf("LoadSettings: %v", err)
	}
	if settings.DatabaseURL != "postgres://user:secret@db/molotalk" {
		t.Errorf("database URL = %q", settings.DatabaseURL)
	}
	if settings.SigningKeyFile != "/run/secrets/auth-jwt-signing-key" {
		t.Errorf("signing key file = %q", settings.SigningKeyFile)
	}
}

func TestLoadSettingsRequiresBothVariables(t *testing.T) {
	t.Parallel()

	for _, missing := range []string{auth.DatabaseURLVariable, auth.SigningKeyFileVariable} {
		values := map[string]string{
			auth.DatabaseURLVariable:    "postgres://user:secret@db/molotalk",
			auth.SigningKeyFileVariable: "/run/secrets/key",
		}
		delete(values, missing)

		_, err := auth.LoadSettings(func(key string) string { return values[key] })
		if err == nil {
			t.Fatalf("LoadSettings succeeded without %s", missing)
		}
		if !strings.Contains(err.Error(), missing) {
			t.Errorf("error = %q, want the variable name %s", err, missing)
		}
		for _, secret := range []string{"postgres://user:secret@db/molotalk", "/run/secrets/key"} {
			if strings.Contains(err.Error(), secret) {
				t.Errorf("error leaks a configured value: %q", err)
			}
		}
	}
}
