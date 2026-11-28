// Package api подключает прикладной модуль Auth к публичному Connect RPC.
package api

import (
	"context"
	"errors"
	"log/slog"
	"strings"

	"connectrpc.com/connect/v2"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"

	molotalkv1 "github.com/MoloTeam/MoloTalk/backend/gen/molotalk/v1"
	"github.com/MoloTeam/MoloTalk/backend/gen/molotalk/v1/molotalkv1connect"
	"github.com/MoloTeam/MoloTalk/backend/internal/auth"
	"github.com/MoloTeam/MoloTalk/backend/internal/platform/requestid"
)

// Service — прикладные операции Auth, которые обслуживает транспорт.
type Service interface {
	Register(ctx context.Context, input auth.RegisterInput) (auth.TokenPair, error)
	Refresh(ctx context.Context, refreshToken string) (auth.TokenPair, error)
	GetMe(ctx context.Context, claims auth.Claims) (auth.Profile, error)
	Logout(ctx context.Context, claims auth.Claims, allSessions bool) error
	Authenticate(ctx context.Context, token string) (auth.Claims, error)
}

// Handler реализует AuthService поверх прикладного модуля.
type Handler struct {
	molotalkv1connect.UnimplementedAuthServiceHandler
	service Service
	logger  *slog.Logger
}

// New создаёт Connect-обработчик Auth.
func New(service Service, logger *slog.Logger) *Handler {
	return &Handler{service: service, logger: logger}
}

// Register создаёт аккаунт с первоначальным сеансом.
func (h *Handler) Register(ctx context.Context, request *molotalkv1.RegisterRequest) (*molotalkv1.RegisterResponse, error) {
	pair, err := h.service.Register(ctx, auth.RegisterInput{
		Identifier:  request.GetIdentifier(),
		DisplayName: request.GetDisplayName(),
		Password:    request.GetPassword(),
	})
	if err != nil {
		return nil, h.mapError(ctx, err)
	}
	return &molotalkv1.RegisterResponse{Tokens: sessionTokens(pair)}, nil
}

// RefreshToken ротирует refresh token и продлевает сеанс.
func (h *Handler) RefreshToken(ctx context.Context, request *molotalkv1.RefreshTokenRequest) (*molotalkv1.RefreshTokenResponse, error) {
	pair, err := h.service.Refresh(ctx, request.GetRefreshToken())
	if err != nil {
		return nil, h.mapError(ctx, err)
	}
	return &molotalkv1.RefreshTokenResponse{Tokens: sessionTokens(pair)}, nil
}

// GetMe возвращает собственный профиль по проверенному access JWT.
func (h *Handler) GetMe(ctx context.Context, _ *emptypb.Empty) (*molotalkv1.GetMeResponse, error) {
	claims, err := h.authenticate(ctx)
	if err != nil {
		return nil, err
	}
	profile, err := h.service.GetMe(ctx, claims)
	if err != nil {
		return nil, h.mapError(ctx, err)
	}
	return &molotalkv1.GetMeResponse{User: user(profile)}, nil
}

// Logout отзывает текущий сеанс или все сеансы пользователя.
func (h *Handler) Logout(ctx context.Context, request *molotalkv1.LogoutRequest) (*emptypb.Empty, error) {
	claims, err := h.authenticate(ctx)
	if err != nil {
		return nil, err
	}
	if err := h.service.Logout(ctx, claims, request.GetAllSessions()); err != nil {
		return nil, h.mapError(ctx, err)
	}
	return &emptypb.Empty{}, nil
}

func (h *Handler) authenticate(ctx context.Context) (auth.Claims, error) {
	token, ok := bearerToken(requestHeader(ctx, "Authorization"))
	if !ok {
		return auth.Claims{}, connect.NewError(connect.CodeUnauthenticated, "unauthenticated")
	}
	claims, err := h.service.Authenticate(ctx, token)
	if err != nil {
		return auth.Claims{}, h.mapError(ctx, err)
	}
	return claims, nil
}

func requestHeader(ctx context.Context, name string) string {
	info, ok := connect.CallInfoForServerContext(ctx)
	if !ok {
		return ""
	}
	return info.RequestHeader().Get(name)
}

func bearerToken(header string) (string, bool) {
	scheme, token, ok := strings.Cut(strings.TrimSpace(header), " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") {
		return "", false
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return "", false
	}
	return token, true
}

func sessionTokens(pair auth.TokenPair) *molotalkv1.SessionTokens {
	return &molotalkv1.SessionTokens{
		User:             user(pair.Profile),
		SessionId:        pair.SessionID,
		AccessToken:      pair.AccessToken,
		RefreshToken:     pair.RefreshToken,
		AccessExpiresAt:  timestamppb.New(pair.AccessExpiresAt),
		RefreshExpiresAt: timestamppb.New(pair.RefreshExpiresAt),
	}
}

func user(profile auth.Profile) *molotalkv1.User {
	result := &molotalkv1.User{
		Id:          profile.ID,
		DisplayName: profile.DisplayName,
	}
	if profile.Identifier != "" {
		result.Identifier = proto.String(profile.Identifier)
	}
	if profile.AvatarAttachmentID != nil {
		result.AvatarAttachmentId = proto.String(*profile.AvatarAttachmentID)
	}
	return result
}

func (h *Handler) mapError(ctx context.Context, err error) error {
	var policy *auth.PasswordPolicyError
	if errors.As(err, &policy) {
		return connect.NewError(connect.CodeInvalidArgument, policy.Error())
	}
	var invalid *auth.InvalidInputError
	if errors.As(err, &invalid) {
		return connect.NewError(connect.CodeInvalidArgument, invalid.Error())
	}
	switch {
	case errors.Is(err, auth.ErrIdentifierTaken):
		return connect.NewError(connect.CodeAlreadyExists, "identifier already exists")
	case errors.Is(err, auth.ErrUnauthenticated), errors.Is(err, auth.ErrProfileNotFound):
		return connect.NewError(connect.CodeUnauthenticated, "unauthenticated")
	case errors.Is(err, auth.ErrStorageUnavailable):
		h.logFailure(ctx, "postgres_unavailable")
		return connect.NewError(connect.CodeUnavailable, "service temporarily unavailable")
	default:
		h.logFailure(ctx, "internal_error")
		return err
	}
}

func (h *Handler) logFailure(ctx context.Context, reason string) {
	if h.logger == nil {
		return
	}
	id, _ := requestid.FromContext(ctx)
	h.logger.WarnContext(ctx, "auth operation failed", "request_id", id, "reason", reason)
}
