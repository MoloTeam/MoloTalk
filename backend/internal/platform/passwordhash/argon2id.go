// Package passwordhash реализует порт хеширования паролей модуля Auth на
// argon2id со случайной солью. Хеш хранится в самодостаточной строке формата
// PHC: параметры и соль лежат рядом с результатом, поэтому параметры можно
// усиливать, не ломая уже сохранённые хеши.
package passwordhash

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

// ErrInvalidHash означает, что строка не является хешом argon2id в поддерживаемом
// формате или содержит недопустимые параметры. Текст ошибки не содержит ни
// пароль, ни проверяемую строку.
var ErrInvalidHash = errors.New("passwordhash: invalid argon2id hash")

// Границы параметров, принимаемых при проверке. Они защищают от строки с
// чрезмерными параметрами, которая заставила бы сервер выделить гигабайты
// памяти при проверке пароля.
const (
	minSaltLength = 8
	minKeyLength  = 16
	maxKeyLength  = 128
	maxMemoryKiB  = 256 * 1024
	maxIterations = 10
	maxThreads    = 16
)

// Params задаёт параметры argon2id.
type Params struct {
	// MemoryKiB — объём памяти в килобайтах.
	MemoryKiB uint32
	// Iterations — число проходов.
	Iterations uint32
	// Parallelism — степень параллелизма.
	Parallelism uint8
	// SaltLength — длина случайной соли в байтах.
	SaltLength uint32
	// KeyLength — длина результата в байтах.
	KeyLength uint32
}

// DefaultParams возвращает параметры по второй рекомендации RFC 9106
// (64 МиБ, 3 прохода, 4 потока). Один вызов Hash или Verify занимает порядка
// десятков миллисекунд и 64 МиБ памяти, поэтому число одновременных
// регистраций и входов стоит ограничивать на уровне сервиса.
func DefaultParams() Params {
	return Params{
		MemoryKiB:   64 * 1024,
		Iterations:  3,
		Parallelism: 4,
		SaltLength:  16,
		KeyLength:   32,
	}
}

// Argon2id хеширует и проверяет пароли. Удовлетворяет порту auth.PasswordHasher.
type Argon2id struct {
	params Params
	salt   io.Reader
}

// NewArgon2id создаёт хешер с заданными параметрами. Соль берётся из
// crypto/rand. Некорректные параметры заменяются безопасными значениями по
// умолчанию, чтобы хешер нельзя было случайно настроить на слабый или
// пустой результат.
func NewArgon2id(params Params) *Argon2id {
	return newArgon2id(params, rand.Reader)
}

func newArgon2id(params Params, salt io.Reader) *Argon2id {
	defaults := DefaultParams()
	if params.MemoryKiB == 0 {
		params.MemoryKiB = defaults.MemoryKiB
	}
	if params.Iterations == 0 {
		params.Iterations = defaults.Iterations
	}
	if params.Parallelism == 0 {
		params.Parallelism = defaults.Parallelism
	}
	if params.SaltLength < minSaltLength {
		params.SaltLength = defaults.SaltLength
	}
	if params.KeyLength < minKeyLength {
		params.KeyLength = defaults.KeyLength
	}
	return &Argon2id{params: params, salt: salt}
}

// Hash возвращает хеш пароля в формате PHC:
// $argon2id$v=19$m=<память>,t=<проходы>,p=<потоки>$<соль>$<хеш>.
// Каждый вызов использует новую случайную соль.
func (h *Argon2id) Hash(ctx context.Context, password string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}

	salt := make([]byte, h.params.SaltLength)
	if _, err := io.ReadFull(h.salt, salt); err != nil {
		return "", fmt.Errorf("passwordhash: read salt: %w", err)
	}

	key := argon2.IDKey([]byte(password), salt, h.params.Iterations, h.params.MemoryKiB, h.params.Parallelism, h.params.KeyLength)
	return encode(h.params, salt, key), nil
}

// Verify проверяет пароль по хешу. Параметры берутся из самого хеша, поэтому
// хеши, созданные с прежними параметрами, продолжают проверяться. Неверный
// пароль возвращает (false, nil); строку неподдерживаемого формата — (false,
// ErrInvalidHash).
func (h *Argon2id) Verify(ctx context.Context, password, encoded string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}

	params, salt, want, err := decode(encoded)
	if err != nil {
		return false, err
	}

	got := argon2.IDKey([]byte(password), salt, params.Iterations, params.MemoryKiB, params.Parallelism, uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1, nil
}

func encode(params Params, salt, key []byte) string {
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, params.MemoryKiB, params.Iterations, params.Parallelism,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key))
}

func decode(encoded string) (Params, []byte, []byte, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[0] != "" || parts[1] != "argon2id" {
		return Params{}, nil, nil, ErrInvalidHash
	}

	if parts[2] != "v="+strconv.Itoa(argon2.Version) {
		return Params{}, nil, nil, ErrInvalidHash
	}

	params, err := parseParams(parts[3])
	if err != nil {
		return Params{}, nil, nil, err
	}

	encoding := base64.RawStdEncoding.Strict()
	salt, err := encoding.DecodeString(parts[4])
	if err != nil || len(salt) < minSaltLength {
		return Params{}, nil, nil, ErrInvalidHash
	}
	key, err := encoding.DecodeString(parts[5])
	if err != nil || len(key) < minKeyLength || len(key) > maxKeyLength {
		return Params{}, nil, nil, ErrInvalidHash
	}

	params.SaltLength = uint32(len(salt))
	params.KeyLength = uint32(len(key))
	return params, salt, key, nil
}

// parseParams разбирает строку вида m=65536,t=3,p=4 строго в этом порядке.
func parseParams(raw string) (Params, error) {
	fields := strings.Split(raw, ",")
	if len(fields) != 3 {
		return Params{}, ErrInvalidHash
	}

	memory, ok := parseField(fields[0], "m=")
	if !ok || memory == 0 || memory > maxMemoryKiB {
		return Params{}, ErrInvalidHash
	}
	iterations, ok := parseField(fields[1], "t=")
	if !ok || iterations == 0 || iterations > maxIterations {
		return Params{}, ErrInvalidHash
	}
	threads, ok := parseField(fields[2], "p=")
	if !ok || threads == 0 || threads > maxThreads {
		return Params{}, ErrInvalidHash
	}
	// Требование argon2: память не меньше 8 КиБ на поток.
	if memory < 8*threads {
		return Params{}, ErrInvalidHash
	}

	return Params{
		MemoryKiB:   uint32(memory),
		Iterations:  uint32(iterations),
		Parallelism: uint8(threads),
	}, nil
}

func parseField(field, prefix string) (uint64, bool) {
	value, found := strings.CutPrefix(field, prefix)
	if !found || value == "" {
		return 0, false
	}
	number, err := strconv.ParseUint(value, 10, 32)
	if err != nil {
		return 0, false
	}
	return number, true
}
