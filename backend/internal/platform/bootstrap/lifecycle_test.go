package bootstrap_test

import (
	"context"
	"errors"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/MoloTeam/MoloTalk/backend/internal/platform/bootstrap"
	"github.com/MoloTeam/MoloTalk/backend/internal/platform/config"
	"github.com/MoloTeam/MoloTalk/backend/internal/platform/httpserver"
)

func TestStartupFailuresReleaseResources(t *testing.T) {
	t.Parallel()

	const marker = "private-adapter-marker"
	failure := errors.New("listener failed")
	adapterFailure := errors.New(marker)
	tests := []struct {
		name      string
		want      error
		operation string
		listens   bool
	}{
		{"configure", adapterFailure, "configure application", false},
		{"register", adapterFailure, "register application routes", false},
		{"conflict", nil, "register application routes", false},
		{"listen", failure, "listen on", true},
		{"serve", failure, "listener failed", true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			listener := &failingListener{acceptError: failure, closed: make(chan struct{})}
			ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
			defer cancel()
			listened := false
			cleaned := make(chan struct{})
			err := bootstrap.Run(ctx, bootstrap.Options{
				Service: config.Auth,
				Getenv:  func(string) string { return "" },
				Configure: func(runtime context.Context) (bootstrap.Application, error) {
					app := bootstrap.Application{
						Register: func(routes *httpserver.Routes) error {
							switch tc.name {
							case "register":
								return adapterFailure
							case "conflict":
								routes.Handle("GET /healthz", http.NotFoundHandler())
							}
							return nil
						},
						Shutdown: func(cleanup context.Context) error {
							defer close(cleaned)
							if runtime.Err() != context.Canceled || cleanup.Err() != nil {
								return errors.New("cleanup did not get a live budget after runtime cancellation")
							}
							if _, ok := cleanup.Deadline(); !ok {
								return errors.New("cleanup did not get a deadline")
							}
							return nil
						},
					}
					if tc.name == "configure" {
						return app, adapterFailure
					}
					return app, nil
				},
				Listen: func(string, string) (net.Listener, error) {
					listened = true
					if tc.name == "listen" {
						return listener, failure
					}
					return listener, nil
				},
			})

			if err == nil || !strings.Contains(err.Error(), tc.operation) || (tc.want != nil && !errors.Is(err, tc.want)) {
				t.Fatalf("Run = %v, want %s failure", err, tc.operation)
			}
			if errors.Is(err, context.DeadlineExceeded) || ctx.Err() != nil {
				t.Fatalf("startup failure waited for the parent timeout: %v", err)
			}
			if (tc.name == "configure" || tc.name == "register") && strings.Contains(err.Error(), marker) {
				t.Error("startup error exposes adapter details")
			}
			if strings.Contains(err.Error(), "shut down application") {
				t.Fatalf("cleanup received an invalid context: %v", err)
			}
			if listened != tc.listens {
				t.Errorf("listener opened = %v, want %v", listened, tc.listens)
			}
			select {
			case <-cleaned:
			default:
				t.Error("application was not cleaned up")
			}
			if listened {
				select {
				case <-listener.closed:
				default:
					t.Error("listener was not closed")
				}
			}
		})
	}
}

func TestApplicationCanStartAgainAfterShutdown(t *testing.T) {
	t.Parallel()

	started := 0
	stopped := 0
	options := bootstrap.Options{
		Configure: func(context.Context) (bootstrap.Application, error) {
			started++
			return bootstrap.Application{
				Shutdown: func(context.Context) error {
					stopped++
					return nil
				},
			}, nil
		},
	}
	for run := 1; run <= 2; run++ {
		server := runApplication(t, options)
		assertStatus(t, server, "/healthz", http.StatusOK)
		server.stop(t)
		if started != run || stopped != run {
			t.Fatalf("run %d: configure/cleanup counts = %d/%d", run, started, stopped)
		}
		if conn, err := net.DialTimeout("tcp", server.address, time.Second); err == nil {
			_ = conn.Close()
			t.Fatal("the previous listener accepts connections")
		}
	}
}

type failingListener struct {
	acceptError error
	closed      chan struct{}
}

func (l *failingListener) Accept() (net.Conn, error) { return nil, l.acceptError }
func (l *failingListener) Addr() net.Addr            { return &net.TCPAddr{} }
func (l *failingListener) Close() error {
	close(l.closed)
	return nil
}
