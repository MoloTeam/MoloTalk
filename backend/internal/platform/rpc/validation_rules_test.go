package rpc_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"buf.build/go/protovalidate"
		"google.golang.org/protobuf/proto"

	molotalkv1 "github.com/MoloTeam/MoloTalk/backend/gen/molotalk/v1"
	"github.com/MoloTeam/MoloTalk/backend/gen/molotalk/v1/molotalkv1connect"
	"github.com/MoloTeam/MoloTalk/backend/internal/platform/rpc"
)

func TestFieldBoundaries(t *testing.T) {
	t.Parallel()

	const ulid = "01ARZ3NDEKTSV4RRFFQ69G5FA1"
	text := func(length int) string { return strings.Repeat("ж", length) }

	checks := []struct {
		name    string
		valid   bool
		message proto.Message
	}{
		{"display name 1", true, &molotalkv1.User{Id: ulid, DisplayName: "a"}},
		{"display name 64", true, &molotalkv1.User{Id: ulid, DisplayName: strings.Repeat("a", 64)}},
		{"display name 65", false, &molotalkv1.User{Id: ulid, DisplayName: strings.Repeat("a", 65)}},
		{"display name empty", false, &molotalkv1.User{Id: ulid, DisplayName: ""}},
		{"group name 100", true, &molotalkv1.CreateGroupChatRequest{ClientRequestId: ulid, Name: strings.Repeat("n", 100)}},
		{"group name 101", false, &molotalkv1.CreateGroupChatRequest{ClientRequestId: ulid, Name: strings.Repeat("n", 101)}},
		{"identifier 254", true, &molotalkv1.RegisterRequest{Identifier: strings.Repeat("a", 254), DisplayName: "A", Password: "secret1"}},
		{"identifier 255", false, &molotalkv1.RegisterRequest{Identifier: strings.Repeat("a", 255), DisplayName: "A", Password: "secret1"}},
		{"identifier uppercase", false, &molotalkv1.RegisterRequest{Identifier: "Alice", DisplayName: "A", Password: "secret1"}},
		{"identifier cyrillic lowercase", true, &molotalkv1.RegisterRequest{Identifier: "алиса", DisplayName: "A", Password: "secret1"}},
		{"password 5", false, &molotalkv1.RegisterRequest{Identifier: "alice", DisplayName: "A", Password: "12345"}},
		{"password 6", true, &molotalkv1.RegisterRequest{Identifier: "alice", DisplayName: "A", Password: "123456"}},
		{"text 4096 multibyte", true, &molotalkv1.SendMessageRequest{ChatId: ulid, ClientMessageId: ulid, Text: pointer(text(4096))}},
		{"text 4097 multibyte", false, &molotalkv1.SendMessageRequest{ChatId: ulid, ClientMessageId: ulid, Text: pointer(text(4097))}},
		{"limit 0", false, &molotalkv1.GetMessagesRequest{ChatId: ulid, Mode: molotalkv1.MessageQueryMode_MESSAGE_QUERY_MODE_LATEST, Limit: pointer(int32(0))}},
		{"limit 101", false, &molotalkv1.GetMessagesRequest{ChatId: ulid, Mode: molotalkv1.MessageQueryMode_MESSAGE_QUERY_MODE_LATEST, Limit: pointer(int32(101))}},
		{"limit absent", true, &molotalkv1.GetMessagesRequest{ChatId: ulid, Mode: molotalkv1.MessageQueryMode_MESSAGE_QUERY_MODE_LATEST}},
		{"search limit 51", false, &molotalkv1.SearchUsersRequest{Query: "ali", Limit: pointer(int32(51))}},
		{"attachments 5", true, &molotalkv1.SendMessageRequest{ChatId: ulid, ClientMessageId: ulid, Text: pointer("hi"), AttachmentIds: []string{ulid, ulid, ulid, ulid, ulid}}},
		{"attachments 6", false, &molotalkv1.SendMessageRequest{ChatId: ulid, ClientMessageId: ulid, Text: pointer("hi"), AttachmentIds: []string{ulid, ulid, ulid, ulid, ulid, ulid}}},
		{"invalid ulid", false, &molotalkv1.User{Id: "01ARZ3NDEKTSV4RRFFQ69G5FAI", DisplayName: "A"}},
		{"enum zero", false, &molotalkv1.Chat{Id: ulid, Kind: molotalkv1.ChatKind_CHAT_KIND_UNSPECIFIED}},
	}

	for _, tc := range checks {
		t.Run(tc.name, func(t *testing.T) {
			err := protovalidate.Validate(tc.message)
			if tc.valid && err != nil {
				t.Fatalf("expected valid, got %v", err)
			}
			if !tc.valid && err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func pointer[T any](value T) *T { return &value }

type emptySearchHandler struct {
	molotalkv1connect.UnimplementedAuthServiceHandler
}

func (h *emptySearchHandler) SearchUsers(_ context.Context, _ *molotalkv1.SearchUsersRequest) (*molotalkv1.SearchUsersResponse, error) {
	return &molotalkv1.SearchUsersResponse{}, nil
}

func TestEmptyArrayIsPresentInJSON(t *testing.T) {
	t.Parallel()

	server := rpc.NewServer()
	molotalkv1connect.RegisterAuthServiceHandler(server, &emptySearchHandler{})

	mux := http.NewServeMux()
	rpc.Mount(mux, server)
	httpServer := httptest.NewServer(mux)
	defer httpServer.Close()

	request, err := http.NewRequest(http.MethodPost, httpServer.URL+"/molotalk.v1.AuthService/SearchUsers", strings.NewReader(`{"query":"alice"}`))
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Connect-Protocol-Version", "1")

	response, err := httpServer.Client().Do(request)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	defer response.Body.Close()
	payload, _ := io.ReadAll(response.Body)

	var decoded map[string]any
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("decode: %v", err)
	}
	users, ok := decoded["users"].([]any)
	if !ok {
		t.Fatalf("users is not an array: %s", payload)
	}
	if len(users) != 0 {
		t.Fatalf("users has %d items", len(users))
	}
}
