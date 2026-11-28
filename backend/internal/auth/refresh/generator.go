// Package refresh создаёт непрозрачные refresh token и их хеши.
package refresh

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"

	"github.com/MoloTeam/MoloTalk/backend/internal/auth"
)

const tokenBytes = 32

// Generator создаёт refresh token из криптографического источника.
type Generator struct {
	reader io.Reader
}

// New создаёт генератор с crypto/rand.
func New() *Generator {
	return &Generator{reader: rand.Reader}
}

// NewWithReader создаёт генератор с заданным источником для проверок.
func NewWithReader(reader io.Reader) *Generator {
	return &Generator{reader: reader}
}

// New возвращает непрозрачный токен и его SHA-256 хеш.
func (g *Generator) New() (string, string, error) {
	buffer := make([]byte, tokenBytes)
	if _, err := io.ReadFull(g.reader, buffer); err != nil {
		return "", "", fmt.Errorf("refresh: generate token: %w", err)
	}
	token := base64.RawURLEncoding.EncodeToString(buffer)
	return token, auth.HashRefreshToken(token), nil
}
