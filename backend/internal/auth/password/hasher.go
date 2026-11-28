// Package password реализует Argon2id-хеширование паролей в формате PHC.
package password

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"golang.org/x/crypto/argon2"
)

const (
	// MinKeyLength — минимальная длина производного ключа.
	MinKeyLength uint32 = 16
	// MinMemory — минимальный профиль памяти Argon2id.
	MinMemory uint32 = 8 * 1024
	// MaxKeyLength — максимальная длина производного ключа.
	MaxKeyLength uint32 = 64
	// MinSaltLength — минимальная длина соли.
	MinSaltLength uint32 = 8
	// MaxSaltLength — максимальная длина соли.
	MaxSaltLength uint32 = 64

	maxMemory      uint32 = 1 << 20 // 1 GiB: защита от вредоносного encoded-хеша.
	maxIterations  uint32 = 10
	maxParallelism uint8  = 16
)

// Params задаёт профиль Argon2id.
type Params struct {
	Memory      uint32
	Iterations  uint32
	Parallelism uint8
	SaltLength  uint32
	KeyLength   uint32
}

// DefaultParams — профиль Auth: 64 MiB памяти, 3 итерации, 2 потока,
// соль 16 байт и ключ 32 байта. Параметры и соль хранятся вместе с хешем.
var DefaultParams = Params{
	Memory:      64 * 1024,
	Iterations:  3,
	Parallelism: 2,
	SaltLength:  16,
	KeyLength:   32,
}

// Hasher хеширует и проверяет пароли Argon2id.
type Hasher struct {
	params Params
	reader io.Reader
}

// New создаёт хешер с заданным профилем и криптографическим источником соли.
func New(params Params) *Hasher {
	return &Hasher{params: params, reader: rand.Reader}
}

// NewWithReader создаёт хешер с заданным источником соли. Используется в
// проверках детерминированной соли.
func NewWithReader(params Params, reader io.Reader) *Hasher {
	return &Hasher{params: params, reader: reader}
}

// Hash возвращает PHC-строку Argon2id с параметрами и солью.
func (h *Hasher) Hash(ctx context.Context, password string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if err := validateParams(h.params); err != nil {
		return "", err
	}

	salt := make([]byte, h.params.SaltLength)
	if _, err := io.ReadFull(h.reader, salt); err != nil {
		return "", fmt.Errorf("password: generate salt: %w", err)
	}
	key := argon2.IDKey([]byte(password), salt, h.params.Iterations, h.params.Memory, h.params.Parallelism, h.params.KeyLength)

	return fmt.Sprintf(
		"$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version,
		h.params.Memory,
		h.params.Iterations,
		h.params.Parallelism,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key),
	), nil
}

// Verify сравнивает пароль с сохранённым PHC-хешем за постоянное время.
// Неверный пароль даёт false и отсутствие ошибки; повреждённый хеш — ошибку.
func (h *Hasher) Verify(ctx context.Context, password, encoded string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}

	params, salt, expected, err := decode(encoded)
	if err != nil {
		return false, err
	}

	actual := argon2.IDKey([]byte(password), salt, params.Iterations, params.Memory, params.Parallelism, uint32(len(expected)))
	return subtle.ConstantTimeCompare(actual, expected) == 1, nil
}

func validateParams(params Params) error {
	switch {
	case params.Memory < MinMemory || params.Memory > maxMemory:
		return errors.New("password: invalid memory parameter")
	case params.Iterations == 0 || params.Iterations > maxIterations:
		return errors.New("password: invalid iterations parameter")
	case params.Parallelism == 0 || params.Parallelism > maxParallelism:
		return errors.New("password: invalid parallelism parameter")
	case params.SaltLength < MinSaltLength || params.SaltLength > MaxSaltLength:
		return errors.New("password: invalid salt length")
	case params.KeyLength < MinKeyLength || params.KeyLength > MaxKeyLength:
		return errors.New("password: invalid key length")
	}
	return nil
}

func decode(encoded string) (Params, []byte, []byte, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[0] != "" || parts[1] != "argon2id" {
		return Params{}, nil, nil, errors.New("password: malformed hash")
	}
	if parts[2] != fmt.Sprintf("v=%d", argon2.Version) {
		return Params{}, nil, nil, errors.New("password: unsupported argon2 version")
	}

	var params Params
	for _, field := range strings.Split(parts[3], ",") {
		name, value, ok := strings.Cut(field, "=")
		if !ok {
			return Params{}, nil, nil, errors.New("password: malformed hash parameters")
		}
		parsed, err := strconv.ParseUint(value, 10, 32)
		if err != nil {
			return Params{}, nil, nil, errors.New("password: malformed hash parameters")
		}
		switch name {
		case "m":
			params.Memory = uint32(parsed)
		case "t":
			params.Iterations = uint32(parsed)
		case "p":
			if parsed > 255 {
				return Params{}, nil, nil, errors.New("password: malformed hash parameters")
			}
			params.Parallelism = uint8(parsed)
		default:
			return Params{}, nil, nil, errors.New("password: malformed hash parameters")
		}
	}

	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil || uint32(len(salt)) < MinSaltLength || uint32(len(salt)) > MaxSaltLength {
		return Params{}, nil, nil, errors.New("password: malformed salt")
	}
	key, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil || uint32(len(key)) < MinKeyLength || uint32(len(key)) > MaxKeyLength {
		return Params{}, nil, nil, errors.New("password: malformed key")
	}
	params.SaltLength = uint32(len(salt))
	params.KeyLength = uint32(len(key))
	if err := validateParams(params); err != nil {
		return Params{}, nil, nil, err
	}

	return params, salt, key, nil
}
