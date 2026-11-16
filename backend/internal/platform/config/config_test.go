package config_test

import (
	"strings"
	"testing"
	"time"

	"github.com/MoloTeam/MoloTalk/backend/internal/platform/config"
)

func environment(values map[string]string) func(string) string {
	return func(key string) string {
		return values[key]
	}
}

func TestDefaultsForEveryProcess(t *testing.T) {
	t.Parallel()

	cases := []struct {
		service     config.Service
		defaultPort int
	}{
		{config.Auth, 8081},
		{config.Chat, 8082},
		{config.Media, 8083},
		{config.Presence, 8084},
		{config.Gateway, 8085},
	}

	for _, tc := range cases {
		cfg, err := config.Load(tc.service, environment(nil))
		if err != nil {
			t.Fatalf("%s: load defaults: %v", tc.service.Name, err)
		}
		if cfg.Host != "127.0.0.1" {
			t.Errorf("%s: host = %q, want 127.0.0.1", tc.service.Name, cfg.Host)
		}
		if cfg.Port != tc.defaultPort {
			t.Errorf("%s: port = %d, want %d", tc.service.Name, cfg.Port, tc.defaultPort)
		}
		if cfg.ReadinessTimeout != time.Second {
			t.Errorf("%s: timeout = %v, want 1s", tc.service.Name, cfg.ReadinessTimeout)
		}
	}
}

func TestExplicitAddressAndPort(t *testing.T) {
	t.Parallel()

	cfg, err := config.Load(config.Gateway, environment(map[string]string{
		"HTTP_HOST":         "0.0.0.0",
		"GATEWAY_HTTP_PORT": "9000",
	}))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.Host != "0.0.0.0" {
		t.Errorf("host = %q, want 0.0.0.0", cfg.Host)
	}
	if cfg.Addr() != "0.0.0.0:9000" {
		t.Errorf("addr = %q, want 0.0.0.0:9000", cfg.Addr())
	}
}

func TestPortBounds(t *testing.T) {
	t.Parallel()

	for _, value := range []string{"1", "65535"} {
		if _, err := config.Load(config.Auth, environment(map[string]string{"AUTH_HTTP_PORT": value})); err != nil {
			t.Errorf("port %q: unexpected error: %v", value, err)
		}
	}

	for _, value := range []string{"0", "65536", "-1", "abc", "80.5", " 08x "} {
		_, err := config.Load(config.Auth, environment(map[string]string{"AUTH_HTTP_PORT": value}))
		if err == nil {
			t.Errorf("port %q: expected error", value)
			continue
		}
		if !strings.Contains(err.Error(), "AUTH_HTTP_PORT") {
			t.Errorf("port %q: error does not name the variable: %v", value, err)
		}
	}
}

func TestReadinessTimeoutBounds(t *testing.T) {
	t.Parallel()

	for _, value := range []string{"100ms", "5s", "1s"} {
		cfg, err := config.Load(config.Auth, environment(map[string]string{"READINESS_TIMEOUT": value}))
		if err != nil {
			t.Errorf("timeout %q: unexpected error: %v", value, err)
			continue
		}
		want, _ := time.ParseDuration(value)
		if cfg.ReadinessTimeout != want {
			t.Errorf("timeout %q: got %v", value, cfg.ReadinessTimeout)
		}
	}

	for _, value := range []string{"99ms", "5001ms", "10s", "garbage"} {
		_, err := config.Load(config.Auth, environment(map[string]string{"READINESS_TIMEOUT": value}))
		if err == nil {
			t.Errorf("timeout %q: expected error", value)
			continue
		}
		if !strings.Contains(err.Error(), "READINESS_TIMEOUT") {
			t.Errorf("timeout %q: error does not name the variable: %v", value, err)
		}
	}
}

func TestInvalidValueIsNotReported(t *testing.T) {
	t.Parallel()

	_, err := config.Load(config.Auth, environment(map[string]string{"AUTH_HTTP_PORT": "70000"}))
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), "70000") {
		t.Errorf("error leaks the value: %v", err)
	}
}
