package httpserver_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/MoloTeam/MoloTalk/backend/internal/platform/httpserver"
)

func TestOptionalRoutes(t *testing.T) {
	t.Parallel()

	server, _ := newServer(t, time.Second, nil)
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	})

	tests := []struct {
		name     string
		register func(*httpserver.Routes) error
		wantApp  int
	}{
		{"nil", nil, http.StatusNotFound},
		{"fallback", func(r *httpserver.Routes) error { r.Handle("/", next); return nil }, http.StatusAccepted},
		{"host-specific", func(r *httpserver.Routes) error { r.Handle("GET example.com/app", next); return nil }, http.StatusAccepted},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			handler, err := server.HandlerWithRoutes(tc.register)
			if err != nil {
				t.Fatalf("mount: %v", err)
			}
			for _, check := range []struct {
				method, path string
				status       int
			}{
				{http.MethodGet, "/app", tc.wantApp},
				{http.MethodGet, "/healthz", http.StatusOK},
				{http.MethodHead, "/readyz", http.StatusServiceUnavailable},
			} {
				response := httptest.NewRecorder()
				handler.ServeHTTP(response, httptest.NewRequest(check.method, "http://example.com"+check.path, nil))
				if response.Code != check.status {
					t.Errorf("%s %s = %d, want %d", check.method, check.path, response.Code, check.status)
				}
			}
		})
	}
}

func TestInvalidRouteRegistrationFails(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name     string
		patterns []string
	}{
		{"same operational route", []string{"GET /healthz"}},
		{"other method on reserved path", []string{"POST /readyz"}},
		{"escaped reserved path", []string{"POST /%68ealthz"}},
		{"HEAD shadows GET", []string{"HEAD /healthz"}},
		{"host wildcard shadows readiness", []string{"GET example.com/{operation}"}},
		{"duplicate app route", []string{"/app", "/app"}},
		{"invalid pattern", []string{"GET /{invalid"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server, _ := newServer(t, time.Second, nil)
			handler, err := server.HandlerWithRoutes(func(r *httpserver.Routes) error {
				for _, pattern := range tc.patterns {
					r.Handle(pattern, http.NotFoundHandler())
				}
				return nil
			})
			if err == nil || handler != nil {
				t.Fatalf("mount = %v, %v; want no handler and an error", handler, err)
			}
		})
	}
}

func TestMountReturnsCallbackError(t *testing.T) {
	t.Parallel()

	server, _ := newServer(t, time.Second, nil)
	want := errors.New("register callback failed")
	handler, err := server.HandlerWithRoutes(func(*httpserver.Routes) error { return want })
	if !errors.Is(err, want) || handler != nil {
		t.Fatalf("mount = %v, %v; want no handler and the callback error", handler, err)
	}
}
