//go:build integration && !windows

package main

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/MoloTeam/MoloTalk/backend/internal/platform/migrations"
	"github.com/MoloTeam/MoloTalk/backend/internal/testenv"
)

func TestMain(m *testing.M) {
	code := m.Run()
	testenv.Terminate()
	os.Exit(code)
}

func TestAuthProcessHandlesSIGTERM(t *testing.T) {
	const childEnv = "MOLOTALK_TEST_AUTH_PROCESS"
	if os.Getenv(childEnv) == "1" {
		main()
		return
	}

	dsn := testenv.Shared(t).NewDatabaseDSN(t)
	if err := migrations.Run(t.Context(), dsn); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
	_, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate test signing key: %v", err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatalf("encode test signing key: %v", err)
	}
	keyFile := filepath.Join(t.TempDir(), "auth-key.pem")
	if err := os.WriteFile(keyFile, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}), 0600); err != nil {
		t.Fatalf("write test signing key: %v", err)
	}

	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve auth HTTP port: %v", err)
	}
	address := listener.Addr().String()
	_, port, err := net.SplitHostPort(address)
	if err != nil {
		listener.Close()
		t.Fatalf("read reserved port: %v", err)
	}
	if err := listener.Close(); err != nil {
		t.Fatalf("release port for Auth: %v", err)
	}

	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	var output bytes.Buffer
	proc := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestAuthProcessHandlesSIGTERM$")
	proc.Env = append(os.Environ(), childEnv+"=1", "DATABASE_URL="+dsn,
		"AUTH_JWT_SIGNING_KEY_FILE="+keyFile, "HTTP_HOST=127.0.0.1", "AUTH_HTTP_PORT="+port)
	proc.Stdout, proc.Stderr = &output, &output
	if err := proc.Start(); err != nil {
		t.Fatalf("start Auth: %v", err)
	}
	done := make(chan struct{})
	var exitErr error
	go func() {
		exitErr = proc.Wait()
		close(done)
	}()
	defer func() {
		cancel()
		<-done
	}()

	client := &http.Client{Timeout: 250 * time.Millisecond}
	defer client.CloseIdleConnections()
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
waitForReady:
	for {
		select {
		case <-done:
			t.Fatalf("Auth exited before readiness: %v (%s)", exitErr, output.String())
		case <-ctx.Done():
			t.Fatal("Auth never became ready")
		case <-ticker.C:
			response, err := client.Get("http://" + address + "/readyz")
			if err == nil {
				response.Body.Close()
				if response.StatusCode == http.StatusOK {
					break waitForReady
				}
			}
		}
	}

	if err := proc.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatalf("send SIGTERM: %v", err)
	}
	select {
	case <-done:
		if exitErr != nil {
			t.Fatalf("Auth did not shut down cleanly after SIGTERM: %v (%s)", exitErr, output.String())
		}
	case <-ctx.Done():
		t.Fatal("Auth did not exit after SIGTERM")
	}
}
