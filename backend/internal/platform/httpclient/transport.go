// Package httpclient переносит идентификатор операции во внутренние
// HTTP-вызовы.
package httpclient

import (
	"net/http"

	"github.com/MoloTeam/MoloTalk/backend/internal/platform/requestid"
)

// Transport ставит заголовок X-Request-Id из контекста операции на каждый
// исходящий запрос.
type Transport struct {
	Base http.RoundTripper
}

// RoundTrip добавляет идентификатор операции, не изменяя переданный запрос.
func (t Transport) RoundTrip(request *http.Request) (*http.Response, error) {
	base := t.Base
	if base == nil {
		base = http.DefaultTransport
	}

	if id, ok := requestid.FromContext(request.Context()); ok && id != "" {
		request = request.Clone(request.Context())
		request.Header.Set(requestid.Header, id)
	}

	return base.RoundTrip(request)
}
