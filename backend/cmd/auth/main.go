// Команда auth запускает процесс Auth.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"

	molotalkv1connect "github.com/MoloTeam/MoloTalk/backend/gen/molotalk/v1/molotalkv1connect"
	"github.com/MoloTeam/MoloTalk/backend/internal/auth"
	"github.com/MoloTeam/MoloTalk/backend/internal/auth/access"
	"github.com/MoloTeam/MoloTalk/backend/internal/auth/api"
	"github.com/MoloTeam/MoloTalk/backend/internal/auth/password"
	authpostgres "github.com/MoloTeam/MoloTalk/backend/internal/auth/postgres"
	"github.com/MoloTeam/MoloTalk/backend/internal/auth/refresh"
	"github.com/MoloTeam/MoloTalk/backend/internal/platform/bootstrap"
	"github.com/MoloTeam/MoloTalk/backend/internal/platform/clock"
	"github.com/MoloTeam/MoloTalk/backend/internal/platform/config"
	"github.com/MoloTeam/MoloTalk/backend/internal/platform/httpserver"
	"github.com/MoloTeam/MoloTalk/backend/internal/platform/idgen"
	"github.com/MoloTeam/MoloTalk/backend/internal/platform/rpc"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := bootstrap.Run(ctx, bootstrap.Options{
		Service:   config.Auth,
		Getenv:    os.Getenv,
		Listen:    net.Listen,
		Out:       os.Stdout,
		Configure: configure,
	}); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func configure(ctx context.Context) (bootstrap.Application, error) {
	settings, err := auth.LoadSettings(os.Getenv)
	if err != nil {
		return bootstrap.Application{}, err
	}
	signingKey, err := access.LoadSigningKey(settings.SigningKeyFile)
	if err != nil {
		return bootstrap.Application{}, err
	}

	store, err := authpostgres.Open(ctx, settings.DatabaseURL)
	if err != nil {
		return bootstrap.Application{}, err
	}

	clk := clock.System{}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil)).With("service", config.Auth.Name)
	module := auth.New(auth.Deps{
		Accounts:      store,
		Sessions:      store,
		Hasher:        password.New(password.DefaultParams),
		Tokens:        access.NewCodec(signingKey, clk),
		RefreshTokens: refresh.New(),
		Clock:         clk,
		IDs:           idgen.NewSystem(clk),
	})

	return bootstrap.Application{
		Readiness: []httpserver.ReadinessCheck{store},
		Register: func(routes *httpserver.Routes) error {
			server := rpc.NewServer()
			molotalkv1connect.RegisterAuthServiceHandler(server, api.New(module, logger))
			rpc.Mount(routes, server)
			return nil
		},
		Shutdown: func(context.Context) error {
			store.Close()
			return nil
		},
	}, nil
}
