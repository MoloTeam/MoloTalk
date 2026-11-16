// Package httpserver собирает эксплуатационные HTTP-маршруты процесса и
// связывает их с идентификатором операции и журналом.
package httpserver

import (
	"context"
	"net/http"
	"time"

	"github.com/MoloTeam/MoloTalk/backend/internal/platform/idgen"
	"github.com/MoloTeam/MoloTalk/backend/internal/platform/logging"
	"github.com/MoloTeam/MoloTalk/backend/internal/platform/requestid"
)

// ReadinessCheck проверяет готовность зависимости. Метод получает общий
// контекст запроса, поэтому отмена распространяется на адаптер.
type ReadinessCheck interface {
	Check(ctx context.Context) error
}

// Options задаёт зависимости HTTP-сервера процесса.
type Options struct {
	Service string
	Checks  []ReadinessCheck
	Timeout time.Duration
	IDs     idgen.IDGenerator
	Logger  *logging.Logger
}

// Server обслуживает эксплуатационные маршруты процесса.
type Server struct {
	service string
	checks  []ReadinessCheck
	timeout time.Duration
	ids     idgen.IDGenerator
	logger  *logging.Logger
}

// New создаёт сервер эксплуатационных маршрутов.
func New(options Options) *Server {
	timeout := options.Timeout
	if timeout <= 0 {
		timeout = time.Second
	}
	return &Server{
		service: options.Service,
		checks:  options.Checks,
		timeout: timeout,
		ids:     options.IDs,
		logger:  options.Logger,
	}
}

// Handler возвращает маршруты с обработкой идентификатора операции.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.health)
	mux.HandleFunc("GET /readyz", s.ready)
	return s.withRequestID(s.withLogging(mux))
}

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	writeStatus(w, http.StatusOK)
}

func (s *Server) ready(w http.ResponseWriter, r *http.Request) {
	if len(s.checks) == 0 {
		writeStatus(w, http.StatusServiceUnavailable)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), s.timeout)
	defer cancel()

	results := make(chan error, len(s.checks))
	for _, check := range s.checks {
		go func(c ReadinessCheck) {
			results <- c.Check(ctx)
		}(check)
	}

	for range s.checks {
		select {
		case err := <-results:
			if err != nil {
				writeStatus(w, http.StatusServiceUnavailable)
				return
			}
		case <-ctx.Done():
			writeStatus(w, http.StatusServiceUnavailable)
			return
		}
	}

	writeStatus(w, http.StatusOK)
}

// withRequestID выбирает итоговый идентификатор до запуска обработчика и
// переносит его в контекст операции и заголовок ответа.
func (s *Server) withRequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, err := requestid.Resolve(r.Header.Values(requestid.Header), s.ids)
		if err != nil {
			id = ""
		}
		w.Header().Set(requestid.Header, id)
		next.ServeHTTP(w, r.WithContext(requestid.WithContext(r.Context(), id)))
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

// withLogging записывает результат операции после обработки запроса.
func (s *Server) withLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		tracked := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(tracked, r)

		id, _ := requestid.FromContext(r.Context())
		result := logging.ResultOK
		if tracked.status >= http.StatusBadRequest {
			result = logging.ResultError
		}
		s.logger.Write(logging.Record{
			RequestID: id,
			Service:   s.service,
			Operation: operationName(r.URL.Path),
			Result:    result,
			Duration:  time.Since(started),
		})
	})
}

func operationName(path string) string {
	switch path {
	case "/healthz":
		return "health"
	case "/readyz":
		return "readiness"
	default:
		return "http"
	}
}

func writeStatus(w http.ResponseWriter, status int) {
	w.WriteHeader(status)
}
