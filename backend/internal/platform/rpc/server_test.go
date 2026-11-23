package rpc_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"connectrpc.com/connect/v2"
	"connectrpc.com/connect/v2/connecthttp"
		"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"

	molotalkv1 "github.com/MoloTeam/MoloTalk/backend/gen/molotalk/v1"
	"github.com/MoloTeam/MoloTalk/backend/gen/molotalk/v1/molotalkv1connect"
	"github.com/MoloTeam/MoloTalk/backend/internal/platform/rpc"
)

type mediaHandler struct {
	molotalkv1connect.UnimplementedMediaServiceHandler

	prepareCalls atomic.Int64
	secret       string
	domainErr    error
}

func (h *mediaHandler) PrepareUpload(_ context.Context, _ *molotalkv1.PrepareUploadRequest) (*molotalkv1.PrepareUploadResponse, error) {
	h.prepareCalls.Add(1)
	if h.secret != "" {
		return nil, errors.New("dependency failed with " + h.secret)
	}
	if h.domainErr != nil {
		return nil, h.domainErr
	}
	return &molotalkv1.PrepareUploadResponse{
		AttachmentId:    "01ARZ3NDEKTSV4RRFFQ69G5FA1",
		UploadUrl:       "https://s3.local/upload",
		UploadExpiresAt: timestamppb.New(time.Date(2026, time.November, 23, 9, 0, 0, 0, time.UTC)),
	}, nil
}

type authHandler struct {
	molotalkv1connect.UnimplementedAuthServiceHandler

	lastClear *bool
}

func (h *authHandler) UpdateProfile(_ context.Context, request *molotalkv1.UpdateProfileRequest) (*molotalkv1.UpdateProfileResponse, error) {
	h.lastClear = request.ClearAvatar
	return &molotalkv1.UpdateProfileResponse{
		User: &molotalkv1.User{Id: "01ARZ3NDEKTSV4RRFFQ69G5FA1", DisplayName: "Alice"},
	}, nil
}

func (h *authHandler) GetMe(_ context.Context, _ *emptypb.Empty) (*molotalkv1.GetMeResponse, error) {
	return &molotalkv1.GetMeResponse{
		User: &molotalkv1.User{Id: "01ARZ3NDEKTSV4RRFFQ69G5FA1", DisplayName: "Alice"},
	}, nil
}

type handlers struct {
	media *mediaHandler
	auth  *authHandler
}

func newServer(t *testing.T) (*handlers, *httptest.Server) {
	t.Helper()

	state := &handlers{media: &mediaHandler{}, auth: &authHandler{}}

	server := rpc.NewServer()
	molotalkv1connect.RegisterMediaServiceHandler(server, state.media)
	molotalkv1connect.RegisterAuthServiceHandler(server, state.auth)

	mux := http.NewServeMux()
	rpc.Mount(mux, server)

	// Действующий контракт корреляции: заголовок запроса возвращается в ответе.
	handler := http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if id := request.Header.Get("X-Request-Id"); id != "" {
			writer.Header().Set("X-Request-Id", id)
		}
		mux.ServeHTTP(writer, request)
	})

	httpServer := httptest.NewServer(handler)
	t.Cleanup(httpServer.Close)
	return state, httpServer
}

func client(t *testing.T, server *httptest.Server) *connect.Client {
	t.Helper()
	return connect.NewClient(connecthttp.NewTransport(server.Client(), server.URL))
}

func TestValidRequestKeepsCorrelationAndTypedResponse(t *testing.T) {
	state, server := newServer(t)
	mediaClient := molotalkv1connect.NewMediaServiceClient(client(t, server))

	ctx, info := connect.NewClientContext(context.Background())
	info.RequestHeader().Set("X-Request-Id", "abc-123")

	response, err := mediaClient.PrepareUpload(ctx, &molotalkv1.PrepareUploadRequest{
		Purpose:     molotalkv1.AttachmentPurpose_ATTACHMENT_PURPOSE_MESSAGE_ATTACHMENT,
		Filename:    "photo.png",
		ContentType: "image/png",
		SizeBytes:   1024,
	})
	if err != nil {
		t.Fatalf("prepare upload: %v", err)
	}
	if state.media.prepareCalls.Load() != 1 {
		t.Fatalf("handler calls = %d, want 1", state.media.prepareCalls.Load())
	}
	if got := info.ResponseHeader().Get("X-Request-Id"); got != "abc-123" {
		t.Fatalf("response X-Request-Id = %q", got)
	}
	if got := response.UploadExpiresAt.AsTime(); !got.Equal(time.Date(2026, time.November, 23, 9, 0, 0, 0, time.UTC)) {
		t.Fatalf("timestamp = %v", got)
	}
}

func TestRawConnectJSONContract(t *testing.T) {
	_, server := newServer(t)

	request, err := http.NewRequest(http.MethodPost, server.URL+"/molotalk.v1.AuthService/GetMe", strings.NewReader(`{}`))
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Connect-Protocol-Version", "1")
	request.Header.Set("X-Request-Id", "abc-123")

	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	defer response.Body.Close()
	payload, _ := io.ReadAll(response.Body)

	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.StatusCode, payload)
	}
	if got := response.Header.Get("X-Request-Id"); got != "abc-123" {
		t.Fatalf("response X-Request-Id = %q", got)
	}
	if !bytes.Contains(payload, []byte(`"user"`)) {
		t.Fatalf("body has no user: %s", payload)
	}
}

func TestValidationStopsInvalidRequest(t *testing.T) {
	state, server := newServer(t)
	mediaClient := molotalkv1connect.NewMediaServiceClient(client(t, server))

	_, err := mediaClient.PrepareUpload(context.Background(), &molotalkv1.PrepareUploadRequest{
		Purpose:     molotalkv1.AttachmentPurpose_ATTACHMENT_PURPOSE_MESSAGE_ATTACHMENT,
		Filename:    "photo.png",
		ContentType: "image/png",
		SizeBytes:   20971521,
	})
	if err == nil {
		t.Fatal("expected validation error")
	}
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("code = %v, want invalid_argument", connect.CodeOf(err))
	}
	if state.media.prepareCalls.Load() != 0 {
		t.Fatalf("handler was called %d times", state.media.prepareCalls.Load())
	}
	if !strings.Contains(err.Error(), "size_bytes") {
		t.Fatalf("error does not name the field: %v", err)
	}
}

func TestDamagedBodyDoesNotReachHandler(t *testing.T) {
	state, server := newServer(t)

	request, err := http.NewRequest(http.MethodPost, server.URL+"/molotalk.v1.MediaService/PrepareUpload", strings.NewReader(`{"purpose": "not-a-number",`))
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Connect-Protocol-Version", "1")
	request.Header.Set("X-Request-Id", "abc-123")

	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	defer response.Body.Close()

	if response.StatusCode == http.StatusOK {
		t.Fatal("damaged body was accepted")
	}
	if state.media.prepareCalls.Load() != 0 {
		t.Fatalf("handler was called for damaged body")
	}
	if got := response.Header.Get("X-Request-Id"); got != "abc-123" {
		t.Fatalf("error response X-Request-Id = %q", got)
	}
}

func TestDomainCodesArePreserved(t *testing.T) {
	state, server := newServer(t)
	mediaClient := molotalkv1connect.NewMediaServiceClient(client(t, server))

	cases := []connect.Code{
		connect.CodeFailedPrecondition,
		connect.CodeUnauthenticated,
		connect.CodePermissionDenied,
		connect.CodeNotFound,
		connect.CodeAlreadyExists,
		connect.CodeResourceExhausted,
	}
	for _, code := range cases {
		state.media.domainErr = connect.NewError(code, "domain")
		_, err := mediaClient.PrepareUpload(context.Background(), &molotalkv1.PrepareUploadRequest{
			Purpose:     molotalkv1.AttachmentPurpose_ATTACHMENT_PURPOSE_MESSAGE_ATTACHMENT,
			Filename:    "photo.png",
			ContentType: "image/png",
			SizeBytes:   1024,
		})
		if connect.CodeOf(err) != code {
			t.Fatalf("code = %v, want %v", connect.CodeOf(err), code)
		}
	}
}

func TestUnknownErrorBecomesSafeInternal(t *testing.T) {
	state, server := newServer(t)
	const secret = "super-secret-dependency"
	state.media.secret = secret
	mediaClient := molotalkv1connect.NewMediaServiceClient(client(t, server))

	_, err := mediaClient.PrepareUpload(context.Background(), &molotalkv1.PrepareUploadRequest{
		Purpose:     molotalkv1.AttachmentPurpose_ATTACHMENT_PURPOSE_MESSAGE_ATTACHMENT,
		Filename:    "photo.png",
		ContentType: "image/png",
		SizeBytes:   1024,
	})
	if connect.CodeOf(err) != connect.CodeInternal {
		t.Fatalf("code = %v, want internal", connect.CodeOf(err))
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("error leaks the secret: %v", err)
	}
}

func TestOptionalPresenceIsDistinct(t *testing.T) {
	state, server := newServer(t)
	authClient := molotalkv1connect.NewAuthServiceClient(client(t, server))

	if _, err := authClient.UpdateProfile(context.Background(), &molotalkv1.UpdateProfileRequest{}); err != nil {
		t.Fatalf("update without clear avatar: %v", err)
	}
	if state.auth.lastClear != nil {
		t.Fatal("absent clear_avatar became present")
	}

	clear := false
	if _, err := authClient.UpdateProfile(context.Background(), &molotalkv1.UpdateProfileRequest{ClearAvatar: &clear}); err != nil {
		t.Fatalf("update with clear avatar: %v", err)
	}
	if state.auth.lastClear == nil {
		t.Fatal("explicit clear_avatar=false was lost")
	}
	if *state.auth.lastClear {
		t.Fatal("clear_avatar changed value")
	}
}
