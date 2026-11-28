//go:build integration

package auth_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/protobuf/encoding/protojson"

	molotalkv1 "github.com/MoloTeam/MoloTalk/backend/gen/molotalk/v1"
	"github.com/MoloTeam/MoloTalk/backend/gen/molotalk/v1/molotalkv1connect"
	"github.com/MoloTeam/MoloTalk/backend/internal/auth"
	"github.com/MoloTeam/MoloTalk/backend/internal/auth/api"
	authpostgres "github.com/MoloTeam/MoloTalk/backend/internal/auth/postgres"
	"github.com/MoloTeam/MoloTalk/backend/internal/platform/httpserver"
	"github.com/MoloTeam/MoloTalk/backend/internal/platform/idgen"
	"github.com/MoloTeam/MoloTalk/backend/internal/platform/logging"
	"github.com/MoloTeam/MoloTalk/backend/internal/platform/rpc"
)

type transport struct {
	server *httptest.Server
	logs   *transportLogs
}

type transportLogs struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (l *transportLogs) Write(data []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.buf.Write(data)
}

func (l *transportLogs) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.buf.String()
}

func newTransport(t *testing.T, f *fixture) *transport {
	t.Helper()

	logs := &transportLogs{}
	httpServer := httpserver.New(httpserver.Options{
		Service: "auth",
		Checks:  []httpserver.ReadinessCheck{f.store},
		Timeout: time.Second,
		IDs:     idgen.NewSystem(f.clock),
		Logger:  logging.New(logs, "auth"),
	})
	handler, err := httpServer.HandlerWithRoutes(func(routes *httpserver.Routes) error {
		server := rpc.NewServer()
		logger := slog.New(slog.NewJSONHandler(logs, nil))
		molotalkv1connect.RegisterAuthServiceHandler(server, api.New(f.module, logger))
		rpc.Mount(routes, server)
		return nil
	})
	if err != nil {
		t.Fatalf("build transport: %v", err)
	}

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return &transport{server: server, logs: logs}
}

func (tr *transport) post(t *testing.T, method, payload, requestID, bearer string) (*http.Response, []byte) {
	t.Helper()

	request, err := http.NewRequest(http.MethodPost, tr.server.URL+"/molotalk.v1.AuthService/"+method, strings.NewReader(payload))
	if err != nil {
		t.Fatalf("build %s request: %v", method, err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Connect-Protocol-Version", "1")
	request.Header.Set("X-Request-Id", requestID)
	if bearer != "" {
		request.Header.Set("Authorization", "Bearer "+bearer)
	}

	response, err := tr.server.Client().Do(request)
	if err != nil {
		t.Fatalf("execute %s: %v", method, err)
	}
	body, err := io.ReadAll(response.Body)
	response.Body.Close()
	if err != nil {
		t.Fatalf("read %s: %v", method, err)
	}
	if got := response.Header.Get("X-Request-Id"); got != requestID {
		t.Fatalf("%s X-Request-Id = %q, want %q", method, got, requestID)
	}
	return response, body
}

func TestSessionLifecycleThroughPublicTransport(t *testing.T) {
	f := newFixture(t)
	transport := newTransport(t, f)

	registerBody := `{"identifier":"alice@example.com","display_name":"Alice","password":"` + testPassword + `"}`
	response, body := transport.post(t, "Register", registerBody, "req-register", "")
	if response.StatusCode != http.StatusOK {
		t.Fatalf("Register status = %d, body %s", response.StatusCode, body)
	}
	var registered molotalkv1.RegisterResponse
	if err := protojson.Unmarshal(body, &registered); err != nil {
		t.Fatalf("decode Register: %v", err)
	}
	tokens := registered.GetTokens()
	if tokens.GetSessionId() == "" || tokens.GetRefreshToken() == "" || tokens.GetAccessToken() == "" {
		t.Fatalf("Register tokens = %+v", tokens)
	}

	response, body = transport.post(t, "GetMe", `{}`, "req-getme", tokens.GetAccessToken())
	if response.StatusCode != http.StatusOK {
		t.Fatalf("GetMe status = %d, body %s", response.StatusCode, body)
	}
	var profile molotalkv1.GetMeResponse
	if err := protojson.Unmarshal(body, &profile); err != nil {
		t.Fatalf("decode GetMe: %v", err)
	}
	if profile.GetUser().GetIdentifier() != "alice@example.com" || profile.GetUser().GetDisplayName() != "Alice" {
		t.Errorf("profile = %+v", profile.GetUser())
	}
	if strings.Contains(string(body), testPassword) {
		t.Fatal("GetMe response contains the password")
	}

	refreshBody := `{"refresh_token":"` + tokens.GetRefreshToken() + `"}`
	response, body = transport.post(t, "RefreshToken", refreshBody, "req-refresh", "")
	if response.StatusCode != http.StatusOK {
		t.Fatalf("RefreshToken status = %d, body %s", response.StatusCode, body)
	}
	var refreshed molotalkv1.RefreshTokenResponse
	if err := protojson.Unmarshal(body, &refreshed); err != nil {
		t.Fatalf("decode RefreshToken: %v", err)
	}
	next := refreshed.GetTokens()
	if next.GetSessionId() != tokens.GetSessionId() {
		t.Errorf("session changed: %q -> %q", tokens.GetSessionId(), next.GetSessionId())
	}

	response, body = transport.post(t, "Logout", `{}`, "req-logout", next.GetAccessToken())
	if response.StatusCode != http.StatusOK {
		t.Fatalf("Logout status = %d, body %s", response.StatusCode, body)
	}

	rejectedBody := `{"refresh_token":"` + next.GetRefreshToken() + `"}`
	response, body = transport.post(t, "RefreshToken", rejectedBody, "req-refresh-rejected", "")
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("Refresh after Logout status = %d, body %s", response.StatusCode, body)
	}
	assertTransportCode(t, body, "unauthenticated")

	if !strings.Contains(transport.logs.String(), "req-register") ||
		!strings.Contains(transport.logs.String(), "req-refresh-rejected") {
		t.Error("operation log does not contain request IDs")
	}
	for _, secret := range []string{testPassword, tokens.GetAccessToken(), tokens.GetRefreshToken(), next.GetRefreshToken()} {
		if strings.Contains(transport.logs.String(), secret) {
			t.Error("operation log contains a password or token")
		}
	}
}

func TestTransportValidationAndErrorContract(t *testing.T) {
	f := newFixture(t)
	transport := newTransport(t, f)

	response, body := transport.post(t, "Register",
		`{"identifier":"Alice","display_name":"Alice","password":"`+testPassword+`"}`,
		"req-uppercase", "")
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("uppercase status = %d, body %s", response.StatusCode, body)
	}
	assertTransportCode(t, body, "invalid_argument")
	if count := f.count(t, `SELECT count(*) FROM users`); count != 0 {
		t.Errorf("rejected request created %d users", count)
	}

	response, body = transport.post(t, "Register",
		`{"identifier":"alice","display_name":"Alice","password":"abcdef"}`,
		"req-policy", "")
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("policy status = %d, body %s", response.StatusCode, body)
	}
	assertTransportCode(t, body, "invalid_argument")
	if strings.Contains(string(body), "abcdef") || !strings.Contains(string(body), "F_Pwd_3") {
		t.Errorf("policy response = %s", body)
	}

	response, body = transport.post(t, "Register",
		`{"identifier":"alice@example.com","display_name":"Alice","password":"`+testPassword+`"}`,
		"req-agree", "")
	if response.StatusCode != http.StatusOK {
		t.Fatalf("baseline Register status = %d, body %s", response.StatusCode, body)
	}
	response, body = transport.post(t, "Register",
		`{"identifier":"alice@example.com","display_name":"Duplicate","password":"`+testPassword+`"}`,
		"req-conflict", "")
	if response.StatusCode != http.StatusConflict {
		t.Fatalf("conflict status = %d, body %s", response.StatusCode, body)
	}
	assertTransportCode(t, body, "already_exists")

	response, body = transport.post(t, "GetMe", `{}`, "req-no-bearer", "")
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("missing Bearer status = %d, body %s", response.StatusCode, body)
	}
	assertTransportCode(t, body, "unauthenticated")
}

func TestTransportDatabaseUnavailableIsSafeAndCorrelated(t *testing.T) {
	f := newFixture(t)
	pair := f.register(t, "alice@example.com")
	const sensitiveDetail = "test-only-sensitive-driver-detail"
	config := f.pool.Config()
	config.ConnConfig.DialFunc = func(context.Context, string, string) (net.Conn, error) {
		return nil, errors.New(sensitiveDetail)
	}
	pool, err := pgxpool.NewWithConfig(t.Context(), config)
	if err != nil {
		t.Fatalf("construct unavailable pool: %v", err)
	}
	t.Cleanup(pool.Close)
	f.store = authpostgres.New(pool)
	f.module = auth.New(auth.Deps{Accounts: f.store, Tokens: f.codec})
	transport := newTransport(t, f)

	const requestID = "req-db-outage"
	response, body := transport.post(t, "GetMe", `{}`, requestID, pair.AccessToken)
	if response.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("GetMe status = %d, want 503", response.StatusCode)
	}
	assertTransportCode(t, body, "unavailable")
	logs := transport.logs.String()
	if strings.Contains(string(body), sensitiveDetail) || strings.Contains(logs, sensitiveDetail) {
		t.Fatal("driver detail leaked from the transport")
	}
	for _, line := range strings.Split(logs, "\n") {
		var entry struct {
			RequestID string `json:"request_id"`
			Reason    string `json:"reason"`
		}
		if json.Unmarshal([]byte(line), &entry) == nil && entry.RequestID == requestID && entry.Reason == "postgres_unavailable" {
			return
		}
	}
	t.Fatal("missing safe PostgreSQL failure reason for the request ID")
}

func assertTransportCode(t *testing.T, body []byte, want string) {
	t.Helper()

	var failure struct {
		Code string
	}
	if err := json.Unmarshal(body, &failure); err != nil {
		t.Fatalf("decode Connect error: %v (%s)", err, body)
	}
	if failure.Code != want {
		t.Fatalf("Connect code = %q, want %q (%s)", failure.Code, want, body)
	}
}
