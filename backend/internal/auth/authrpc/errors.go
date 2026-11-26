// Package authrpc связывает прикладные ошибки Auth с кодами Connect. Доменный
// пакет auth не зависит от транспорта: отображение выполняется здесь.
package authrpc

import (
	"errors"

	"connectrpc.com/connect/v2"

	"github.com/MoloTeam/MoloTalk/backend/internal/auth"
)

const identifierTakenMessage = "Логин (e-mail) уже используется другой учётной записью"

// Error переводит ошибку модуля Auth в *connect.Error:
//
//   - *auth.PasswordPolicyError и *auth.ValidationError → invalid_argument;
//   - auth.ErrIdentifierTaken → already_exists;
//   - ошибка, уже являющаяся *connect.Error, возвращается без изменений;
//   - прочие ошибки возвращаются как есть, а rpc.ErrorInterceptor заменяет их
//     безопасным internal без текста зависимости.
//
// Текст invalid_argument берётся из сообщений домена и не содержит пароль.
func Error(err error) error {
	if err == nil {
		return nil
	}

	var existing *connect.Error
	if errors.As(err, &existing) {
		return err
	}

	var policy *auth.PasswordPolicyError
	var validation *auth.ValidationError
	switch {
	case errors.As(err, &policy):
		return connect.NewError(connect.CodeInvalidArgument, policy.Error())
	case errors.As(err, &validation):
		return connect.NewError(connect.CodeInvalidArgument, validation.Error())
	case errors.Is(err, auth.ErrIdentifierTaken):
		return connect.NewError(connect.CodeAlreadyExists, identifierTakenMessage)
	default:
		return err
	}
}
