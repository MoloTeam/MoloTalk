// Package ulid создаёт и разбирает идентификаторы ULID.
package ulid

import (
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"strings"
)

const (
	// Length — длина строкового представления идентификатора.
	Length = 26
	// Alphabet — алфавит Crockford base32.
	Alphabet = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"
)

// ErrInvalidID возвращается, когда строка не является допустимым ULID.
var ErrInvalidID = errors.New("invalid ulid")

// ID — 128-битный идентификатор.
type ID [16]byte

// New возвращает новый ULID из криптографически стойкого источника случайности.
func New() (ID, error) {
	var id ID
	if _, err := rand.Read(id[:]); err != nil {
		return ID{}, fmt.Errorf("read random bytes: %w", err)
	}
	return id, nil
}

// String кодирует идентификатор в 26 символов Crockford base32.
func (id ID) String() string {
	value := new(big.Int).SetBytes(id[:])
	base := big.NewInt(32)
	buf := make([]byte, Length)
	for i := Length - 1; i >= 0; i-- {
		digit := new(big.Int).Mod(value, base)
		buf[i] = Alphabet[digit.Int64()]
		value.Div(value, base)
	}
	return string(buf)
}

// Parse разбирает строковое представление ULID.
func Parse(value string) (ID, error) {
	if len(value) != Length {
		return ID{}, ErrInvalidID
	}
	var id ID
	result := new(big.Int)
	base := big.NewInt(32)
	for _, char := range strings.ToUpper(value) {
		index := strings.IndexRune(Alphabet, char)
		if index < 0 {
			return ID{}, ErrInvalidID
		}
		result.Mul(result, base)
		result.Add(result, big.NewInt(int64(index)))
	}
	bytes := result.Bytes()
	if len(bytes) > len(id) {
		return ID{}, ErrInvalidID
	}
	copy(id[len(id)-len(bytes):], bytes)
	return id, nil
}

// IsValid сообщает, является ли строка допустимым ULID.
func IsValid(value string) bool {
	_, err := Parse(value)
	return err == nil
}
