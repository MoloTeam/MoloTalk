package httpclient_test

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/MoloTeam/MoloTalk/backend/internal/platform/httpclient"
	"github.com/MoloTeam/MoloTalk/backend/internal/platform/requestid"
)

type recordingTransport struct {
	received *http.Request
}

func (r *recordingTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	r.received = request
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader("ok")),
		Header:     make(http.Header),
	}, nil
}

func newRequest(t *testing.T) *http.Request {
	t.Helper()

	request, err := http.NewRequest(http.MethodGet, "http://chat.internal/members", nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	return request
}

func TestInternalCallCarriesOperationIdentifier(t *testing.T) {
	t.Parallel()

	base := &recordingTransport{}
	transport := httpclient.Transport{Base: base}

	ctx := requestid.WithContext(context.Background(), "operation-identifier")
	response, err := transport.RoundTrip(newRequest(t).WithContext(ctx))
	if err != nil {
		t.Fatalf("round trip: %v", err)
	}
	response.Body.Close()

	if got := base.received.Header.Get(requestid.Header); got != "operation-identifier" {
		t.Fatalf("X-Request-Id = %q, want operation-identifier", got)
	}
}

func TestRequestWithoutIdentifierIsSentUnchanged(t *testing.T) {
	t.Parallel()

	base := &recordingTransport{}
	transport := httpclient.Transport{Base: base}

	response, err := transport.RoundTrip(newRequest(t))
	if err != nil {
		t.Fatalf("round trip: %v", err)
	}
	response.Body.Close()

	if got := base.received.Header.Get(requestid.Header); got != "" {
		t.Fatalf("X-Request-Id = %q, want no header", got)
	}
}

func TestRequestIsNotMutated(t *testing.T) {
	t.Parallel()

	base := &recordingTransport{}
	transport := httpclient.Transport{Base: base}

	request := newRequest(t)
	ctx := requestid.WithContext(context.Background(), "operation-identifier")

	response, err := transport.RoundTrip(request.WithContext(ctx))
	if err != nil {
		t.Fatalf("round trip: %v", err)
	}
	response.Body.Close()

	if got := request.Header.Get(requestid.Header); got != "" {
		t.Fatalf("the caller request was mutated: X-Request-Id = %q", got)
	}
}
