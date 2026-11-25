// Package rpc содержит минимальные общие настройки Connect-сервера:
// декларативную валидацию protovalidate и безопасное отображение ошибок.
package rpc

import (
	"context"
	"errors"
	"strings"

	validatepb "buf.build/gen/go/bufbuild/protovalidate/protocolbuffers/go/buf/validate"
	"buf.build/go/protovalidate"
	"connectrpc.com/connect/v2"
	"connectrpc.com/connect/v2/connectproto"
	"google.golang.org/protobuf/proto"
)

// NewServer создаёт Connect-сервер с валидацией запросов и безопасными ошибками.
func NewServer() *connect.Server {
	return connect.NewServer(ValidationInterceptor, ErrorInterceptor)
}

// ValidationInterceptor проверяет каждое входящее сообщение до прикладного
// метода и возвращает штатные protovalidate violations как invalid_argument.
func ValidationInterceptor(next connect.ServerFunc) connect.ServerFunc {
	return func(ctx context.Context, spec connect.Spec, stream connect.ServerStream) error {
		return next(ctx, spec, &validatingStream{ServerStream: stream})
	}
}

type validatingStream struct {
	connect.ServerStream
}

func (s *validatingStream) Receive(message any) error {
	if err := s.ServerStream.Receive(message); err != nil {
		return err
	}
	protoMessage, ok := message.(proto.Message)
	if !ok {
		return nil
	}
	if err := protovalidate.Validate(protoMessage); err != nil {
		return validationError(err)
	}
	return nil
}

func validationError(err error) error {
	var validation *protovalidate.ValidationError
	if !errors.As(err, &validation) {
		return connect.NewError(connect.CodeInvalidArgument, "invalid request")
	}

	parts := make([]string, 0, len(validation.Violations))
	violations := &validatepb.Violations{Violations: make([]*validatepb.Violation, 0, len(validation.Violations))}
	for _, violation := range validation.Violations {
		field := protovalidate.FieldPathString(violation.Proto.GetField())
		parts = append(parts, field+": "+violation.Proto.GetMessage())
		violations.Violations = append(violations.Violations, violation.Proto)
	}

	connectError := connect.NewError(connect.CodeInvalidArgument, strings.Join(parts, "; "))
	// Клиент получает структурированные нарушения, а не только текст.
	if detail, err := connectproto.NewErrorDetail(violations); err == nil {
		connectError = connectError.WithDetail(detail)
	}
	return connectError
}

// ErrorInterceptor сохраняет доменные коды Connect и заменяет прочие ошибки
// безопасным internal без исходного текста зависимости.
func ErrorInterceptor(next connect.ServerFunc) connect.ServerFunc {
	return func(ctx context.Context, spec connect.Spec, stream connect.ServerStream) error {
		err := next(ctx, spec, stream)
		if err == nil {
			return nil
		}

		var connectError *connect.Error
		if errors.As(err, &connectError) {
			return connectError
		}
		return connect.NewError(connect.CodeInternal, "internal error")
	}
}
