// Package logging пишет структурированную запись результата операции.
package logging

import (
	"encoding/json"
	"io"
	"sync"
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

type entry struct {
	RequestID  string `json:"request_id"`
	Service    string `json:"service"`
	Operation  string `json:"operation"`
	Result     string `json:"result"`
	DurationMS int64  `json:"duration_ms"`
	UserID     string `json:"user_id,omitempty"`
	ChatID     string `json:"chat_id,omitempty"`
	MessageID  string `json:"message_id,omitempty"`
}

// Logger пишет записи в формате JSON, по одной в строке.
type Logger struct {
	mu      sync.Mutex
	out     io.Writer
	service string
}

// New создаёт журнал процесса.
func New(out io.Writer, service string) *Logger {
	return &Logger{out: out, service: service}
}

// Write записывает результат операции.
func (l *Logger) Write(record Record) {
	if l == nil || l.out == nil {
		return
	}
	service := record.Service
	if service == "" {
		service = l.service
	}
	line, err := json.Marshal(entry{
		RequestID:  record.RequestID,
		Service:    service,
		Operation:  record.Operation,
		Result:     record.Result,
		DurationMS: record.Duration.Milliseconds(),
		UserID:     record.UserID,
		ChatID:     record.ChatID,
		MessageID:  record.MessageID,
	})
	if err != nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	_, _ = l.out.Write(append(line, '\n'))
}
