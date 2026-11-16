package bootstrap_test

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/MoloTeam/MoloTalk/backend/internal/platform/bootstrap"
	"github.com/MoloTeam/MoloTalk/backend/internal/platform/config"
)

func TestInvalidConfigurationStopsBeforeListen(t *testing.T) {
	t.Parallel()

	listened := false
	err := bootstrap.Run(context.Background(), bootstrap.Options{
		Service: config.Auth,
		Getenv: func(key string) string {
			if key == "AUTH_HTTP_PORT" {
				return "70000"
			}
			return ""
		},
		Listen: func(string, string) (net.Listener, error) {
			listened = true
			return nil, errors.New("listener must not be opened")
		},
		Out: io.Discard,
	})

	if err == nil {
		t.Fatal("expected configuration error")
	}
	if listened {
		t.Fatal("listener was opened despite invalid configuration")
	}
	if !strings.Contains(err.Error(), "AUTH_HTTP_PORT") {
		t.Errorf("error does not name the variable: %v", err)
	}
	if strings.Contains(err.Error(), "70000") {
		t.Errorf("error leaks the value: %v", err)
	}
}

func TestServesOperationalRoutes(t *testing.T) {
	t.Parallel()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- bootstrap.Run(ctx, bootstrap.Options{
			Service: config.Auth,
			Getenv:  func(string) string { return "" },
			Listen:  func(string, string) (net.Listener, error) { return listener, nil },
			Out:     io.Discard,
		})
	}()

	base := "http://" + listener.Addr().String()
	waitForHealth(t, base)

	for _, tc := range []struct {
		path string
		want int
	}{
		{"/healthz", http.StatusOK},
		{"/readyz", http.StatusServiceUnavailable},
	} {
		response, err := http.Get(base + tc.path)
		if err != nil {
			t.Fatalf("GET %s: %v", tc.path, err)
		}
		response.Body.Close()
		if response.StatusCode != tc.want {
			t.Errorf("GET %s = %d, want %d", tc.path, response.StatusCode, tc.want)
		}
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("run: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("process did not stop after cancellation")
	}
}

func waitForHealth(t *testing.T, base string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		response, err := http.Get(base + "/healthz")
		if err == nil {
			response.Body.Close()
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("process did not start serving")
}
