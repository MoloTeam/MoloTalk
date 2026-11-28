package httpserver

import (
	"errors"
	"net/http"
	"net/url"
	"strings"
)

const (
	healthPath = "/healthz"
	readyPath  = "/readyz"
)

// Routes регистрирует прикладные маршруты и сохраняет первую ошибку регистрации.
type Routes struct {
	mux *http.ServeMux
	err error
}

// Handle принимает шаблоны ServeMux и подходит для монтирования Connect.
func (r *Routes) Handle(pattern string, handler http.Handler) {
	if r.err != nil {
		return
	}
	// ServeMux сообщает о некорректном или конфликтующем шаблоне через panic.
	defer func() {
		if recover() != nil {
			r.err = errors.New("httpserver: invalid or conflicting route")
		}
	}()
	r.mux.Handle(pattern, handler)

	rest := pattern
	if i := strings.IndexAny(rest, " \t"); i >= 0 {
		rest = strings.TrimLeft(rest[i+1:], " \t")
	}
	host, suffix, _ := strings.Cut(rest, "/")
	path, _ := url.PathUnescape("/" + suffix)
	for _, reserved := range [...]string{healthPath, readyPath} {
		if path == reserved || r.shadows(pattern, host, reserved) {
			r.err = errors.New("httpserver: reserved route " + reserved)
			return
		}
	}
}

func (r *Routes) shadows(pattern, host, path string) bool {
	// ServeMux отдаёт приоритет шаблонам с host даже над точным путём без host.
	probe := &http.Request{Host: host, URL: &url.URL{Path: path}, Method: http.MethodGet}
	if _, matched := r.mux.Handler(probe); matched == pattern {
		return true
	}
	probe.Method = http.MethodHead
	_, matched := r.mux.Handler(probe)
	return matched == pattern
}
