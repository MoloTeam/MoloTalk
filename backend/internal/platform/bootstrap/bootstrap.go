// Package bootstrap собирает процесс из конфигурации, модуля и слушателя.
package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"slices"
	"time"

	"github.com/MoloTeam/MoloTalk/backend/internal/platform/clock"
	"github.com/MoloTeam/MoloTalk/backend/internal/platform/config"
	"github.com/MoloTeam/MoloTalk/backend/internal/platform/httpserver"
	"github.com/MoloTeam/MoloTalk/backend/internal/platform/idgen"
	"github.com/MoloTeam/MoloTalk/backend/internal/platform/logging"
)

const shutdownTimeout = 5 * time.Second

// Hosted описывает прикладной модуль, который обслуживает процесс.
type Hosted interface {
	Name() string
	Readiness() []httpserver.ReadinessCheck
}

// Application связывает прикладной транспорт и его ресурсы с процессом.
type Application struct {
	Register  func(*httpserver.Routes) error
	Readiness []httpserver.ReadinessCheck

	// Shutdown освобождает ресурсы, включая захваченные транспортом соединения.
	// Он вызывается после отмены контекста Configure и соблюдает свой бюджет.
	Shutdown func(context.Context) error
}

// Options задаёт зависимости точки входа.
type Options struct {
	Service config.Service
	Hosted  Hosted
	Getenv  func(string) string
	Listen  func(network, address string) (net.Listener, error)
	Out     io.Writer
	IDs     idgen.IDGenerator

	// Configure открывает ресурсы до регистрации маршрутов и старта HTTP.
	// При частичной ошибке возвращает Application с Shutdown для уже открытых ресурсов.
	Configure func(context.Context) (Application, error)
}

// Run проверяет настройки, открывает слушатель и обслуживает запросы до
// отмены контекста. Ошибка сборки возвращается до начала обслуживания.
func Run(ctx context.Context, options Options) (runErr error) {
	cfg, err := config.Load(options.Service, options.Getenv)
	if err != nil {
		return err
	}

	runtimeCtx, cancel := context.WithCancel(ctx)
	var app Application
	var server *http.Server
	defer func() {
		cancel()
		if server == nil {
			runErr = errors.Join(runErr, stopProcess(nil, app.Shutdown))
		}
	}()

	if options.Configure != nil && runtimeCtx.Err() == nil {
		app, err = options.Configure(runtimeCtx)
		if err != nil {
			return &lifecycleError{"configure application", err}
		}
	}
	if runtimeCtx.Err() != nil {
		return nil
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

	checks := app.Readiness
	if options.Hosted != nil {
		checks = slices.Concat(options.Hosted.Readiness(), checks)
	}
	handler, err := httpserver.New(httpserver.Options{
		Service: cfg.Service,
		Checks:  checks,
		Timeout: cfg.ReadinessTimeout,
		IDs:     ids,
		Logger:  logging.New(out, cfg.Service),
	}).HandlerWithRoutes(app.Register)
	if err != nil {
		return &lifecycleError{"register application routes", err}
	}
	if runtimeCtx.Err() != nil {
		return nil
	}

	listener, err := listen("tcp", cfg.Addr())
	if err != nil {
		if listener != nil {
			_ = listener.Close()
		}
		return fmt.Errorf("listen on %s: %w", cfg.Addr(), err)
	}
	server = &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		BaseContext:       func(net.Listener) context.Context { return runtimeCtx },
	}

	serveDone := make(chan struct{})
	stopped := make(chan error, 1)
	go func() {
		select {
		case <-runtimeCtx.Done():
		case <-serveDone:
		}
		cancel()
		stopped <- stopProcess(server, app.Shutdown)
	}()

	err = server.Serve(listener)
	close(serveDone)
	if errors.Is(err, http.ErrServerClosed) {
		err = nil
	}
	return errors.Join(err, <-stopped)
}

func stopProcess(server *http.Server, shutdown func(context.Context) error) error {
	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	return stop(ctx, server, shutdown)
}

func stop(ctx context.Context, server *http.Server, shutdown func(context.Context) error) error {
	results := make(chan error, 2)
	pending := 0
	if server != nil {
		pending++
		go func() {
			results <- server.Shutdown(ctx)
		}()
	}
	if shutdown != nil {
		pending++
		go func() {
			err := shutdown(ctx)
			if err != nil {
				err = &lifecycleError{"shut down application", err}
			}
			results <- err
		}()
	}

	var err error
	for range pending {
		select {
		case result := <-results:
			err = errors.Join(err, result)
		case <-ctx.Done():
			err = errors.Join(err, ctx.Err())
			// Shutdown не закрывает активные HTTP-соединения по истечении бюджета.
			if server != nil {
				_ = server.Close()
			}
			return err
		}
	}
	if server != nil && ctx.Err() != nil {
		_ = server.Close()
	}
	return err
}

// lifecycleError скрывает детали адаптера при выводе ошибки запуска из cmd.
type lifecycleError struct {
	operation string
	cause     error
}

func (e *lifecycleError) Error() string { return "bootstrap: " + e.operation + " failed" }
func (e *lifecycleError) Unwrap() error { return e.cause }
