// Package requestid выбирает и переносит идентификатор HTTP-операции.
package requestid

import (
	"context"
	"regexp"

	"github.com/MoloTeam/MoloTalk/backend/internal/platform/idgen"
)

// Header — заголовок с идентификатором операции.
const Header = "X-Request-Id"

// pattern допускает от 1 до 64 символов, начиная с буквы или цифры.
var pattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)

type contextKey struct{}

// Valid сообщает, соответствует ли значение допустимому формату.
func Valid(value string) bool {
	return pattern.MatchString(value)
}

// WithContext помещает идентификатор операции в контекст.
func WithContext(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, contextKey{}, id)
}

// FromContext извлекает идентификатор операции из контекста.
func FromContext(ctx context.Context) (string, bool) {
	id, ok := ctx.Value(contextKey{}).(string)
	return id, ok
}

// Resolve выбирает итоговый идентификатор операции. Допустимое единственное
// значение сохраняется, отсутствующее, пустое, повторное или некорректное
// заменяется новым ULID.
func Resolve(values []string, generator idgen.IDGenerator) (string, error) {
	if len(values) == 1 && Valid(values[0]) {
		return values[0], nil
	}
	generated, err := generator.NewID()
	if err != nil {
		return "", err
	}
	return generated.String(), nil
}
