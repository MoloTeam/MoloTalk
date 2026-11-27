package bootstrap_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/emptypb"

	molotalkv1 "github.com/MoloTeam/MoloTalk/backend/gen/molotalk/v1"
	"github.com/MoloTeam/MoloTalk/backend/gen/molotalk/v1/molotalkv1connect"
	"github.com/MoloTeam/MoloTalk/backend/internal/platform/bootstrap"
	"github.com/MoloTeam/MoloTalk/backend/internal/platform/config"
	"github.com/MoloTeam/MoloTalk/backend/internal/platform/httpserver"
	"github.com/MoloTeam/MoloTalk/backend/internal/platform/requestid"
	"github.com/MoloTeam/MoloTalk/backend/internal/platform/rpc"
)

const testTimeout = 5 * time.Second

type checkFunc func(context.Context) error

func (f checkFunc) Check(ctx context.Context) error { return f(ctx) }

type testAuth struct {
	molotalkv1connect.UnimplementedAuthServiceHandler
}

func (testAuth) GetMe(ctx context.Context, _ *emptypb.Empty) (*molotalkv1.GetMeResponse, error) {
	id, _ := requestid.FromContext(ctx)
	return &molotalkv1.GetMeResponse{User: &molotalkv1.User{DisplayName: id}}, nil
}

func (testAuth) SearchUsers(_ context.Context, request *molotalkv1.SearchUsersRequest) (*molotalkv1.SearchUsersResponse, error) {
	return nil, errors.New("dependency failed for " + request.GetQuery())
}

func TestMountedConnectAndReadiness(t *testing.T) {
	t.Parallel()

	var fails atomic.Bool
	checkCancelled := make(chan struct{}, 1)
	check := checkFunc(func(ctx context.Context) error {
		if !fails.Load() {
			return nil
		}
		<-ctx.Done()
		checkCancelled <- struct{}{}
		return ctx.Err()
	})
	var records bytes.Buffer
	server := runApplication(t, bootstrap.Options{
		Out: &records,
		Getenv: func(key string) string {
			if key == "READINESS_TIMEOUT" {
				return "100ms"
			}
			return ""
		},
		Configure: func(context.Context) (bootstrap.Application, error) {
			return bootstrap.Application{
				Readiness: []httpserver.ReadinessCheck{check},
				Register: func(routes *httpserver.Routes) error {
					connectServer := rpc.NewServer()
					molotalkv1connect.RegisterAuthServiceHandler(connectServer, testAuth{})
					rpc.Mount(routes, connectServer)
					return nil
				},
			}, nil
		},
	})

	assertStatus(t, server, "/healthz", http.StatusOK)
	assertStatus(t, server, "/readyz", http.StatusOK)
	fails.Store(true)
	assertStatus(t, server, "/readyz", http.StatusServiceUnavailable)
	select {
	case <-checkCancelled:
	case <-time.After(testTimeout):
		t.Fatal("readiness timeout did not cancel the check")
	}
	fails.Store(false)
	assertStatus(t, server, "/readyz", http.StatusOK)

	const okID = "connect-success"
	body := postConnect(t, server, "GetMe", "{}", okID, http.StatusOK)
	var profile molotalkv1.GetMeResponse
	if err := protojson.Unmarshal(body, &profile); err != nil {
		t.Fatalf("decode profile: %v", err)
	}
	if got := profile.GetUser().GetDisplayName(); got != okID {
		t.Errorf("request ID seen by Connect handler = %q, want %q", got, okID)
	}

	const failID = "connect-failure"
	const marker = "private-query-marker"
	body = postConnect(t, server, "SearchUsers", "{\"query\":\""+marker+"\"}", failID, http.StatusInternalServerError)
	var failure struct {
		Code    string
		Message string
	}
	if err := json.Unmarshal(body, &failure); err != nil || failure.Code != "internal" || failure.Message != "internal error" {
		t.Fatalf("unsafe or invalid Connect error: %s (%v)", body, err)
	}

	server.stop(t)
	assertLog(t, records.String(), okID, "ok")
	assertLog(t, records.String(), failID, "error")
	for _, secret := range []string{marker, "private-bearer-marker"} {
		if strings.Contains(records.String(), secret) {
			t.Error("request data appeared in the operation log")
		}
	}
}

type writerFunc func([]byte) (int, error)

func (f writerFunc) Write(p []byte) (int, error) { return f(p) }

func TestWebSocketUpgradeAndShutdown(t *testing.T) {
	t.Parallel()

	var records bytes.Buffer
	upgradeLogged := make(chan struct{})
	owned := make(chan net.Conn, 1)
	pending := make(chan struct{})
	requestDone := make(chan struct{})
	server := runApplication(t, bootstrap.Options{
		Out: writerFunc(func(p []byte) (int, error) {
			n, err := records.Write(p)
			if bytes.Contains(p, []byte("\"request_id\":\"ws-handshake\"")) {
				close(upgradeLogged)
			}
			return n, err
		}),
		Configure: func(runtime context.Context) (bootstrap.Application, error) {
			return bootstrap.Application{
				Register: func(routes *httpserver.Routes) error {
					routes.Handle("GET /ws", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						id, _ := requestid.FromContext(r.Context())
						if id != "ws-handshake" {
							t.Errorf("upgrade handler request ID = %q", id)
						}
						upgradeForTest(t, w, owned)
					}))
					routes.Handle("GET /pending", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						close(pending)
						<-r.Context().Done()
						w.WriteHeader(http.StatusNoContent)
						close(requestDone)
					}))
					return nil
				},
				Shutdown: func(ctx context.Context) error {
					if runtime.Err() != context.Canceled {
						return errors.New("runtime context was not cancelled")
					}
					select {
					case conn := <-owned:
						return conn.Close()
					case <-ctx.Done():
						return ctx.Err()
					}
				},
			}, nil
		},
	})
	conn := webSocketHandshake(t, server.address)
	defer conn.Close()
	select {
	case <-upgradeLogged:
	case <-time.After(testTimeout):
		t.Fatal("upgrade was not logged")
	}

	callDone := make(chan error, 1)
	go func() {
		response, err := server.client.Get(server.url("/pending"))
		if err == nil {
			response.Body.Close()
		}
		callDone <- err
	}()
	select {
	case <-pending:
	case <-time.After(testTimeout):
		t.Fatal("in-flight request did not reach the handler")
	}

	server.stop(t)
	select {
	case <-requestDone:
	default:
		t.Fatal("shutdown did not cancel the in-flight HTTP request")
	}
	select {
	case err := <-callDone:
		if err != nil {
			t.Fatalf("in-flight HTTP request failed to complete: %v", err)
		}
	case <-time.After(testTimeout):
		t.Fatal("in-flight HTTP request did not complete")
	}
	var data [1]byte
	if _, err := conn.Read(data[:]); !errors.Is(err, io.EOF) {
		t.Fatalf("upgraded connection was not closed by the owner: %v", err)
	}
	assertLog(t, records.String(), "ws-handshake", "ok")
}

func upgradeForTest(t *testing.T, w http.ResponseWriter, owned chan<- net.Conn) {
	t.Helper()
	h, ok := w.(http.Hijacker)
	if !ok {
		t.Error("wrapped response does not support direct Hijacker assertion")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	conn, rw, err := h.Hijack()
	if err != nil {
		t.Errorf("hijack: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// Ответ для фиксированного ключа из примера RFC 6455.
	w.Header().Set("Upgrade", "websocket")
	w.Header().Set("Connection", "Upgrade")
	w.Header().Set("Sec-WebSocket-Accept", "s3pPLMBiTxaQ9kYGzzhZRbK+xOo=")
	if _, err := rw.WriteString("HTTP/1.1 101 Switching Protocols\r\n"); err == nil {
		err = w.Header().Write(rw)
		if err == nil {
			_, err = rw.WriteString("\r\n")
		}
		if err == nil {
			err = rw.Flush()
		}
		if err != nil {
			t.Errorf("write handshake headers: %v", err)
		}
	} else {
		t.Errorf("write handshake status: %v", err)
	}
	owned <- conn
}

func webSocketHandshake(t *testing.T, address string) net.Conn {
	t.Helper()
	conn, err := net.DialTimeout("tcp", address, testTimeout)
	if err != nil {
		t.Fatalf("dial WebSocket: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	if err := conn.SetDeadline(time.Now().Add(2 * testTimeout)); err != nil {
		t.Fatal(err)
	}

	_, err = fmt.Fprintf(conn, "GET /ws HTTP/1.1\r\nHost: %s\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Version: 13\r\nSec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==\r\nX-Request-Id: ws-handshake\r\n\r\n", address)
	if err != nil {
		t.Fatalf("write handshake: %v", err)
	}
	reader := bufio.NewReader(conn)
	response, err := http.ReadResponse(reader, nil)
	if err != nil {
		t.Fatalf("read handshake: %v", err)
	}
	if response.StatusCode != http.StatusSwitchingProtocols ||
		response.Header.Get("Sec-WebSocket-Accept") != "s3pPLMBiTxaQ9kYGzzhZRbK+xOo=" ||
		response.Header.Get("Upgrade") != "websocket" || response.Header.Get("Connection") != "Upgrade" {
		t.Fatalf("invalid handshake: %s, %v", response.Status, response.Header)
	}
	if ids := response.Header.Values("X-Request-Id"); len(ids) != 1 || ids[0] != "ws-handshake" {
		t.Fatalf("handshake X-Request-Id = %q", ids)
	}
	if reader.Buffered() != 0 {
		t.Fatal("unexpected bytes after the handshake")
	}
	return conn
}

type loopback struct {
	address string
	client  *http.Client
	cancel  context.CancelFunc
	done    chan struct{}
	result  error
}

func runApplication(t *testing.T, options bootstrap.Options) *loopback {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = listener.Close() })

	ctx, cancel := context.WithCancel(context.Background())
	server := &loopback{
		address: listener.Addr().String(),
		client:  &http.Client{Timeout: testTimeout},
		cancel:  cancel,
		done:    make(chan struct{}),
	}
	options.Service = config.Auth
	if options.Getenv == nil {
		options.Getenv = func(string) string { return "" }
	}
	options.Listen = func(string, string) (net.Listener, error) { return listener, nil }
	go func() {
		server.result = bootstrap.Run(ctx, options)
		close(server.done)
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-server.done:
		case <-time.After(2 * testTimeout):
			t.Error("loopback server did not exit")
		}
	})
	return server
}

func (s *loopback) url(path string) string { return "http://" + s.address + path }

func (s *loopback) stop(t *testing.T) {
	t.Helper()
	s.cancel()
	select {
	case <-s.done:
		if s.result != nil {
			t.Fatalf("stop application: %v", s.result)
		}
	case <-time.After(testTimeout):
		t.Fatal("application did not stop")
	}
}

func assertStatus(t *testing.T, s *loopback, path string, want int) {
	t.Helper()
	response, err := s.client.Get(s.url(path))
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	defer response.Body.Close()
	if response.StatusCode != want {
		t.Errorf("GET %s = %d, want %d", path, response.StatusCode, want)
	}
}

func postConnect(t *testing.T, s *loopback, method, body, id string, want int) []byte {
	t.Helper()
	request, err := http.NewRequest(http.MethodPost, s.url("/molotalk.v1.AuthService/"+method), strings.NewReader(body))
	if err != nil {
		t.Fatalf("build %s request: %v", method, err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Connect-Protocol-Version", "1")
	request.Header.Set("X-Request-Id", id)
	request.Header.Set("Authorization", "Bearer private-bearer-marker")
	response, err := s.client.Do(request)
	if err != nil {
		t.Fatalf("execute %s: %v", method, err)
	}
	defer response.Body.Close()
	payload, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read %s: %v", method, err)
	}
	if response.StatusCode != want || response.Header.Get("X-Request-Id") != id {
		t.Fatalf("%s status/ID = %d/%q, body %s", method, response.StatusCode, response.Header.Get("X-Request-Id"), payload)
	}
	return payload
}

func assertLog(t *testing.T, raw, id, want string) {
	t.Helper()
	scanner := bufio.NewScanner(strings.NewReader(raw))
	for scanner.Scan() {
		var record struct {
			RequestID string `json:"request_id"`
			Operation string
			Result    string
		}
		if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
			t.Fatalf("decode log: %v", err)
		}
		if record.RequestID == id {
			if record.Operation != "http" || record.Result != want {
				t.Errorf("log for %s = %+v, want http/%s", id, record, want)
			}
			return
		}
	}
	t.Errorf("no log for %s, scanner error %v", id, scanner.Err())
}
