package bootstrap

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestShutdownBudgetForcesClose(t *testing.T) {
	t.Parallel()

	entered := make(chan struct{})
	releaseHTTP := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		close(entered)
		<-releaseHTTP
	}))
	defer server.Close()
	defer close(releaseHTTP)

	callDone := make(chan error, 1)
	go func() {
		response, err := server.Client().Get(server.URL)
		if err == nil {
			response.Body.Close()
		}
		callDone <- err
	}()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("in-flight handler did not start")
	}

	shutdownEntered := make(chan struct{})
	releaseApp := make(chan struct{})
	defer close(releaseApp)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	stopped := make(chan error, 1)
	go func() {
		stopped <- stop(ctx, server.Config, func(ctx context.Context) error {
			close(shutdownEntered)
			<-ctx.Done()
			<-releaseApp
			return ctx.Err()
		})
	}()
	select {
	case <-shutdownEntered:
	case <-time.After(5 * time.Second):
		t.Fatal("application shutdown did not start")
	}
	select {
	case err := <-stopped:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("stop = %v, want deadline exceeded", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("shutdown waited for callbacks beyond its budget")
	}
	select {
	case err := <-callDone:
		if err == nil {
			t.Fatal("in-flight connection should have been force-closed")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("in-flight connection remained open")
	}
}
