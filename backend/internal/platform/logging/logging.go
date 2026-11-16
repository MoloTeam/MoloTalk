// Package logging пишет структурированную запись результата операции.
package logging

import (
	"io"
	"log/slog"
	"time"
)

// Результаты операции.
const (
	ResultOK    = "ok"
	ResultError = "error"
)

// Record — запись результата операции. Поля для паролей, токенов и секретов
// конфигурации отсутствуют намеренно: они не должны попадать в журнал.
type Record struct {
	RequestID string
	Service   string
	Operation string
	Result    string
	Duration  time.Duration
	UserID    string
	ChatID    string
	MessageID string
}

// Logger пишет записи через log/slog в формате JSON.
type Logger struct {
	log *slog.Logger
}

// New создаёт журнал процесса.
func New(out io.Writer, service string) *Logger {
	handler := slog.NewJSONHandler(out, &slog.HandlerOptions{Level: slog.LevelInfo})
	return &Logger{log: slog.New(handler).With(slog.String("service", service))}
}

// Write записывает результат операции.
func (l *Logger) Write(record Record) {
	if l == nil || l.log == nil {
		return
	}

	attributes := []slog.Attr{
		slog.String("request_id", record.RequestID),
		slog.String("operation", record.Operation),
		slog.String("result", record.Result),
		slog.Int64("duration_ms", record.Duration.Milliseconds()),
	}
	if record.UserID != "" {
		attributes = append(attributes, slog.String("user_id", record.UserID))
	}
	if record.ChatID != "" {
		attributes = append(attributes, slog.String("chat_id", record.ChatID))
	}
	if record.MessageID != "" {
		attributes = append(attributes, slog.String("message_id", record.MessageID))
	}

	level := slog.LevelInfo
	if record.Result != ResultOK {
		level = slog.LevelWarn
	}
	l.log.LogAttrs(nil, level, "operation", attributes...)
}
