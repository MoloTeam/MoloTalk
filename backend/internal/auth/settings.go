package auth

import (
	"errors"
	"strings"
)

const (
	// DatabaseURLVariable — DSN PostgreSQL для Auth.
	DatabaseURLVariable = "DATABASE_URL"
	// SigningKeyFileVariable — путь к PKCS#8 Ed25519 ключу подписи access JWT.
	SigningKeyFileVariable = "AUTH_JWT_SIGNING_KEY_FILE"
)

// Settings — проверенные настройки запуска Auth.
type Settings struct {
	DatabaseURL    string
	SigningKeyFile string
}

// LoadSettings читает обязательные настройки Auth. Ошибки называют только
// переменную и не содержат её значения.
func LoadSettings(getenv func(string) string) (Settings, error) {
	databaseURL := strings.TrimSpace(getenv(DatabaseURLVariable))
	if databaseURL == "" {
		return Settings{}, errors.New("auth: DATABASE_URL is required")
	}

	signingKeyFile := strings.TrimSpace(getenv(SigningKeyFileVariable))
	if signingKeyFile == "" {
		return Settings{}, errors.New("auth: AUTH_JWT_SIGNING_KEY_FILE is required")
	}

	return Settings{
		DatabaseURL:    databaseURL,
		SigningKeyFile: signingKeyFile,
	}, nil
}
