package api_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/encoding/protojson"

	molotalkv1 "github.com/MoloTeam/MoloTalk/backend/gen/molotalk/v1"
	"github.com/MoloTeam/MoloTalk/backend/gen/molotalk/v1/molotalkv1connect"
	"github.com/MoloTeam/MoloTalk/backend/internal/auth"
	"github.com/MoloTeam/MoloTalk/backend/internal/auth/api"
	"github.com/MoloTeam/MoloTalk/backend/internal/platform/rpc"
)

type fakeService struct {
	registerCalls int
	register      func(auth.RegisterInput) (auth.TokenPair, error)
	refresh       func(string) (auth.TokenPair, error)
	authenticate  func(string) (auth.Claims, error)
	getMe         func(auth.Claims) (auth.Profile, error)
	logout        func(auth.Claims, bool) error
}

func (f *fakeService) Register(_ context.Context, input auth.RegisterInput) (auth.TokenPair, error) {
	f.registerCalls++
	if f.register == nil {
		return auth.TokenPair{}, errors.New("unexpected Register")
	}
	return f.register(input)
}

func (f *fakeService) Refresh(_ context.Context, token string) (auth.TokenPair, error) {
	if f.refresh == nil {
		return auth.TokenPair{}, errors.New("unexpected Refresh")
	}
	return f.refresh(token)
}

func (f *fakeService) Authenticate(_ context.Context, token string) (auth.Claims, error) {
	if f.authenticate == nil {
		return auth.Claims{}, auth.ErrUnauthenticated
	}
	return f.authenticate(token)
}

func (f *fakeService) GetMe(_ context.Context, claims auth.Claims) (auth.Profile, error) {
	if f.getMe == nil {
		return auth.Profile{}, errors.New("unexpected GetMe")
	}
	return f.getMe(claims)
}

func (f *fakeService) Logout(_ context.Context, claims auth.Claims, all bool) error {
	if f.logout == nil {
		return errors.New("unexpected Logout")
	}
	return f.logout(claims, all)
}

var (
	pairTime = time.Date(2026, time.November, 28, 12, 0, 0, 0, time.UTC)
	pair     = auth.TokenPair{
		Profile: auth.Profile{
			ID:          "01ARZ3NDEKTSV4RRFFQ69G5FA1",
			Identifier:  "alice@example.com",
			DisplayName: "Alice",
		},
		SessionID:        "01ARZ3NDEKTSV4RRFFQ69G5FA2",
		AccessToken:      "access-token",
		RefreshToken:     "refresh-token",
		AccessExpiresAt:  pairTime.Add(15 * time.Minute),
		RefreshExpiresAt: pairTime.Add(30 * 24 * time.Hour),
	}
)

func newServer(t *testing.T, service *fakeService) *httptest.Server {
	t.Helper()

	mux := http.NewServeMux()
	server := rpc.NewServer()
	molotalkv1connect.RegisterAuthServiceHandler(server, api.New(service, nil))
	rpc.Mount(mux, server)

	httpServer := httptest.NewServer(mux)
	t.Cleanup(httpServer.Close)
	return httpServer
}

func TestRegisterReturnsTokens(t *testing.T) {
	t.Parallel()

	service := &fakeService{register: func(input auth.RegisterInput) (auth.TokenPair, error) {
		if input.Identifier != "alice@example.com" || input.DisplayName != "Alice" || input.Password != "Secret1!" {
			t.Errorf("input = %+v", input)
		}
		return pair, nil
	}}
	server := newServer(t, service)

	response, body := post(t, server, "Register", `{"identifier":"alice@example.com","display_name":"Alice","password":"Secret1!"}`, nil)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, body %s", response.StatusCode, body)
	}
	var decoded molotalkv1.RegisterResponse
	if err := protojson.Unmarshal(body, &decoded); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	tokens := decoded.GetTokens()
	if tokens.GetSessionId() != pair.SessionID || tokens.GetRefreshToken() != pair.RefreshToken {
		t.Errorf("tokens = %+v", tokens)
	}
	if tokens.GetUser().GetIdentifier() != "alice@example.com" || tokens.GetUser().GetDisplayName() != "Alice" {
		t.Errorf("user = %+v", tokens.GetUser())
	}
	if tokens.GetAccessExpiresAt().AsTime() != pair.AccessExpiresAt {
		t.Errorf("access expiry = %v", tokens.GetAccessExpiresAt().AsTime())
	}
}

func TestRegisterRejectsProtoInputBeforeService(t *testing.T) {
	t.Parallel()

	service := &fakeService{}
	server := newServer(t, service)

	for name, payload := range map[string]string{
		"uppercase identifier": `{"identifier":"Alice","display_name":"Alice","password":"Secret1!"}`,
		"short password":       `{"identifier":"alice","display_name":"Alice","password":"Aa1!"}`,
		"blank display name":   `{"identifier":"alice","display_name":"","password":"Secret1!"}`,
	} {
		t.Run(name, func(t *testing.T) {
			response, body := post(t, server, "Register", payload, nil)
			if response.StatusCode != http.StatusBadRequest {
				t.Fatalf("status = %d, body %s", response.StatusCode, body)
			}
			assertConnectCode(t, body, "invalid_argument")
		})
	}
	if service.registerCalls != 0 {
		t.Errorf("service Register calls = %d, want 0", service.registerCalls)
	}
}

func TestRegisterMapsDomainErrors(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{name: "conflict", err: auth.ErrIdentifierTaken, status: http.StatusConflict, code: "already_exists"},
		{name: "policy", err: &auth.PasswordPolicyError{Violations: []string{"F_Pwd_3: missing class"}}, status: http.StatusBadRequest, code: "invalid_argument"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			service := &fakeService{register: func(auth.RegisterInput) (auth.TokenPair, error) {
				return auth.TokenPair{}, testCase.err
			}}
			server := newServer(t, service)
			response, body := post(t, server, "Register", `{"identifier":"alice","display_name":"Alice","password":"Secret1!"}`, nil)
			if response.StatusCode != testCase.status {
				t.Fatalf("status = %d, body %s", response.StatusCode, body)
			}
			assertConnectCode(t, body, testCase.code)
			if strings.Contains(string(body), "Secret1!") {
				t.Fatalf("response leaks the password: %s", body)
			}
		})
	}
}

func TestRefreshIsAnonymous(t *testing.T) {
	t.Parallel()

	service := &fakeService{refresh: func(token string) (auth.TokenPair, error) {
		if token != "refresh-token" {
			t.Errorf("refresh token = %q", token)
		}
		return pair, nil
	}}
	server := newServer(t, service)

	response, body := post(t, server, "RefreshToken", `{"refresh_token":"refresh-token"}`, nil)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, body %s", response.StatusCode, body)
	}
	var decoded molotalkv1.RefreshTokenResponse
	if err := protojson.Unmarshal(body, &decoded); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if decoded.GetTokens().GetSessionId() != pair.SessionID {
		t.Errorf("tokens = %+v", decoded.GetTokens())
	}

	service.refresh = func(string) (auth.TokenPair, error) {
		return auth.TokenPair{}, auth.ErrUnauthenticated
	}
	response, body = post(t, server, "RefreshToken", `{"refresh_token":"consumed"}`, nil)
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, body %s", response.StatusCode, body)
	}
	assertConnectCode(t, body, "unauthenticated")
}

func TestGetMeRequiresBearerAndReturnsProfile(t *testing.T) {
	t.Parallel()

	service := &fakeService{
		authenticate: func(token string) (auth.Claims, error) {
			if token != "access" {
				return auth.Claims{}, auth.ErrUnauthenticated
			}
			return auth.Claims{AccountID: pair.Profile.ID, SessionID: pair.SessionID}, nil
		},
		getMe: func(claims auth.Claims) (auth.Profile, error) {
			if claims.AccountID != pair.Profile.ID {
				t.Errorf("claims = %+v", claims)
			}
			return pair.Profile, nil
		},
	}
	server := newServer(t, service)

	response, body := post(t, server, "GetMe", `{}`, map[string]string{"Authorization": "Bearer access"})
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, body %s", response.StatusCode, body)
	}
	var decoded molotalkv1.GetMeResponse
	if err := protojson.Unmarshal(body, &decoded); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if decoded.GetUser().GetIdentifier() != "alice@example.com" {
		t.Errorf("user = %+v", decoded.GetUser())
	}

	for name, header := range map[string]string{
		"missing":       "",
		"wrong scheme":  "Basic access",
		"blank token":   "Bearer ",
		"invalid token": "Bearer invalid",
	} {
		t.Run(name, func(t *testing.T) {
			response, body := post(t, server, "GetMe", `{}`, map[string]string{"Authorization": header})
			if response.StatusCode != http.StatusUnauthorized {
				t.Fatalf("status = %d, body %s", response.StatusCode, body)
			}
			assertConnectCode(t, body, "unauthenticated")
		})
	}
}

func TestLogoutPassesAllSessionsFlag(t *testing.T) {
	t.Parallel()

	var gotAll *bool
	service := &fakeService{
		authenticate: func(string) (auth.Claims, error) {
			return auth.Claims{AccountID: pair.Profile.ID, SessionID: pair.SessionID}, nil
		},
		logout: func(_ auth.Claims, all bool) error {
			gotAll = &all
			return nil
		},
	}
	server := newServer(t, service)

	response, body := post(t, server, "Logout", `{"all_sessions":true}`, map[string]string{"Authorization": "Bearer access"})
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, body %s", response.StatusCode, body)
	}
	if gotAll == nil || !*gotAll {
		t.Fatalf("allSessions = %v, want true", gotAll)
	}

	response, body = post(t, server, "Logout", `{}`, nil)
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, body %s", response.StatusCode, body)
	}
}

func post(t *testing.T, server *httptest.Server, method, payload string, headers map[string]string) (*http.Response, []byte) {
	t.Helper()

	request, err := http.NewRequest(http.MethodPost, server.URL+"/molotalk.v1.AuthService/"+method, strings.NewReader(payload))
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Connect-Protocol-Version", "1")
	for key, value := range headers {
		request.Header.Set(key, value)
	}

	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatalf("execute %s: %v", method, err)
	}
	body, err := io.ReadAll(response.Body)
	response.Body.Close()
	if err != nil {
		t.Fatalf("read %s: %v", method, err)
	}
	return response, body
}

func assertConnectCode(t *testing.T, body []byte, want string) {
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
