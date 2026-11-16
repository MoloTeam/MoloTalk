// Package config читает и проверяет настройки процесса из окружения.
package config

import (
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"
)

const (
	// DefaultHost — адрес HTTP-слушателя по умолчанию.
	DefaultHost = "127.0.0.1"
	// DefaultReadinessTimeout — общий бюджет одного запроса готовности по умолчанию.
	DefaultReadinessTimeout = time.Second
	// MinReadinessTimeout — нижняя допустимая граница бюджета.
	MinReadinessTimeout = 100 * time.Millisecond
	// MaxReadinessTimeout — верхняя допустимая граница бюджета.
	MaxReadinessTimeout = 5 * time.Second

	hostVariable    = "HTTP_HOST"
	timeoutVariable = "READINESS_TIMEOUT"
)

// Service описывает процесс и переменную окружения с номером его порта.
type Service struct {
	Name         string
	PortVariable string
	DefaultPort  int
}

// Процессы каркаса и их порты по умолчанию.
var (
	Auth     = Service{Name: "auth", PortVariable: "AUTH_HTTP_PORT", DefaultPort: 8081}
	Chat     = Service{Name: "chat", PortVariable: "CHAT_HTTP_PORT", DefaultPort: 8082}
	Media    = Service{Name: "media", PortVariable: "MEDIA_HTTP_PORT", DefaultPort: 8083}
	Presence = Service{Name: "presence", PortVariable: "PRESENCE_HTTP_PORT", DefaultPort: 8084}
	Gateway  = Service{Name: "gateway", PortVariable: "GATEWAY_HTTP_PORT", DefaultPort: 8085}
)

// ValueError сообщает о некорректной переменной окружения, не раскрывая её значение.
type ValueError struct {
	Variable string
	Reason   string
}

func (e *ValueError) Error() string {
	return fmt.Sprintf("%s: %s", e.Variable, e.Reason)
}

// Config — проверенные настройки процесса.
type Config struct {
	Service          string
	Host             string
	Port             int
	ReadinessTimeout time.Duration
}

// Addr возвращает адрес HTTP-слушателя.
func (c Config) Addr() string {
	return net.JoinHostPort(c.Host, strconv.Itoa(c.Port))
}

// Load читает настройки процесса. Некорректное значение даёт ошибку с именем
// переменной и без её значения, чтобы секреты не попадали в журнал.
func Load(service Service, getenv func(string) string) (Config, error) {
	cfg := Config{
		Service:          service.Name,
		Host:             DefaultHost,
		Port:             service.DefaultPort,
		ReadinessTimeout: DefaultReadinessTimeout,
	}

	if raw := strings.TrimSpace(getenv(hostVariable)); raw != "" {
		cfg.Host = raw
	}

	if raw := strings.TrimSpace(getenv(service.PortVariable)); raw != "" {
		port, err := strconv.Atoi(raw)
		if err != nil || port < 1 || port > 65535 {
			return Config{}, &ValueError{
				Variable: service.PortVariable,
				Reason:   "must be an integer between 1 and 65535",
			}
		}
		cfg.Port = port
	}

	if raw := strings.TrimSpace(getenv(timeoutVariable)); raw != "" {
		timeout, err := time.ParseDuration(raw)
		if err != nil || timeout < MinReadinessTimeout || timeout > MaxReadinessTimeout {
			return Config{}, &ValueError{
				Variable: timeoutVariable,
				Reason:   "must be a duration between 100ms and 5s",
			}
		}
		cfg.ReadinessTimeout = timeout
	}

	return cfg, nil
}
