// Команда testserver поднимает тестовый Connect-сервер на общих настройках rpc.
// Используется кросс-языковыми проверками TypeScript-клиента.
package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"time"

	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"

	molotalkv1 "github.com/MoloTeam/MoloTalk/backend/gen/molotalk/v1"
	"github.com/MoloTeam/MoloTalk/backend/gen/molotalk/v1/molotalkv1connect"
	"github.com/MoloTeam/MoloTalk/backend/internal/platform/rpc"
)

var timeFixed = time.Date(2026, time.November, 24, 9, 0, 0, 0, time.UTC)

type authHandler struct {
	molotalkv1connect.UnimplementedAuthServiceHandler
}

func (authHandler) SearchUsers(_ context.Context, _ *molotalkv1.SearchUsersRequest) (*molotalkv1.SearchUsersResponse, error) {
	return &molotalkv1.SearchUsersResponse{}, nil
}

func (authHandler) GetMe(_ context.Context, _ *emptypb.Empty) (*molotalkv1.GetMeResponse, error) {
	return &molotalkv1.GetMeResponse{
		User: &molotalkv1.User{Id: "01ARZ3NDEKTSV4RRFFQ69G5FA1", DisplayName: "Alice"},
	}, nil
}

type mediaHandler struct {
	molotalkv1connect.UnimplementedMediaServiceHandler
}

func (mediaHandler) PrepareUpload(_ context.Context, _ *molotalkv1.PrepareUploadRequest) (*molotalkv1.PrepareUploadResponse, error) {
	return &molotalkv1.PrepareUploadResponse{
		AttachmentId:    "01ARZ3NDEKTSV4RRFFQ69G5FA1",
		UploadUrl:       "https://s3.local/upload",
		UploadExpiresAt: timestamppb.New(timeFixed),
	}, nil
}

func main() {
	address := flag.String("addr", "127.0.0.1:0", "listen address")
	flag.Parse()

	listener, err := net.Listen("tcp", *address)
	if err != nil {
		fmt.Fprintf(os.Stderr, "testserver: %v\n", err)
		os.Exit(1)
	}

	server := rpc.NewServer()
	molotalkv1connect.RegisterAuthServiceHandler(server, authHandler{})
	molotalkv1connect.RegisterMediaServiceHandler(server, mediaHandler{})

	mux := http.NewServeMux()
	rpc.Mount(mux, server)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Действующий контракт корреляции: заголовок запроса возвращается в ответе.
	handler := http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if id := request.Header.Get("X-Request-Id"); id != "" {
			writer.Header().Set("X-Request-Id", id)
		}
		mux.ServeHTTP(writer, request)
	})

	// Строка готовности читается запускающей стороной.
	fmt.Printf("listening http://%s\n", listener.Addr().String())
	if err := http.Serve(listener, handler); err != nil {
		fmt.Fprintf(os.Stderr, "testserver: %v\n", err)
		os.Exit(1)
	}
}
