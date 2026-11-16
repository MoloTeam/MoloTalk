// Package bootstrap собирает процесс из конфигурации, модуля и слушателя.
package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	"github.com/MoloTeam/MoloTalk/backend/internal/platform/clock"
	"github.com/MoloTeam/MoloTalk/backend/internal/platform/config"
	"github.com/MoloTeam/MoloTalk/backend/internal/platform/httpserver"
	"github.com/MoloTeam/MoloTalk/backend/internal/platform/idgen"
	"github.com/MoloTeam/MoloTalk/backend/internal/platform/logging"
)

// Hosted описывает прикладной модуль, который обслуживает процесс.
type Hosted interface {
	Name() string
	Readiness() []httpserver.ReadinessCheck
}

// Options задаёт зависимости точки входа.
type Options struct {
	Service config.Service
	Hosted  Hosted
	Getenv  func(string) string
	Listen  func(network, address string) (net.Listener, error)
	Out     io.Writer
	IDs     idgen.IDGenerator
}

// Run проверяет настройки, открывает слушатель и обслуживает запросы до
// отмены контекста. Ошибка конфигурации возвращается до открытия слушателя.
func Run(ctx context.Context, options Options) error {
	cfg, err := config.Load(options.Service, options.Getenv)
	if err != nil {
		return err
	}

	listen := options.Listen
	if listen == nil {
		listen = net.Listen
	}
	ids := options.IDs
	if ids == nil {
		ids = idgen.NewSystem(clock.System{})
	}
	out := options.Out
	if out == nil {
		out = io.Discard
	}

	var checks []httpserver.ReadinessCheck
	if options.Hosted != nil {
		checks = options.Hosted.Readiness()
	}

	server := httpserver.New(httpserver.Options{
		Service: cfg.Service,
		Checks:  checks,
		Timeout: cfg.ReadinessTimeout,
		IDs:     ids,
		Logger:  logging.New(out, cfg.Service),
	})

	listener, err := listen("tcp", cfg.Addr())
	if err != nil {
		return fmt.Errorf("listen on %s: %w", cfg.Addr(), err)
	}

	httpServer := &http.Server{
		Handler:           server.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	shutdown := make(chan struct{})
	go func() {
		defer close(shutdown)
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = httpServer.Shutdown(shutdownCtx)
	}()

	if err := httpServer.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}

	<-shutdown
	return nil
}
