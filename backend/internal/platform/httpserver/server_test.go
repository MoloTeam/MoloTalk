package httpserver_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/MoloTeam/MoloTalk/backend/internal/platform/httpserver"
	"github.com/MoloTeam/MoloTalk/backend/internal/platform/idgen"
	"github.com/MoloTeam/MoloTalk/backend/internal/platform/logging"
	"github.com/MoloTeam/MoloTalk/backend/internal/platform/ulid"
)

type stubCheck struct {
	mu  sync.Mutex
	err error
}

func (c *stubCheck) Check(context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.err
}

func (c *stubCheck) fail(err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.err = err
}

func newServer(t *testing.T, timeout time.Duration, checks []httpserver.ReadinessCheck) (*httpserver.Server, *bytes.Buffer) {
	t.Helper()

	records := &bytes.Buffer{}
	server := httpserver.New(httpserver.Options{
		Service: "auth",
		Checks:  checks,
		Timeout: timeout,
		IDs:     idgen.System{},
		Logger:  logging.New(records, "auth"),
	})
	return server, records
}

func do(t *testing.T, server *httpserver.Server, method, path string, headers map[string][]string) *httptest.ResponseRecorder {
	t.Helper()

	request := httptest.NewRequest(method, path, nil)
	for key, values := range headers {
		for _, value := range values {
			request.Header.Add(key, value)
		}
	}
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, request)
	return recorder
}

func TestFrameworkWithoutChecks(t *testing.T) {
	t.Parallel()

	server, _ := newServer(t, time.Second, nil)

	if got := do(t, server, http.MethodGet, "/healthz", nil).Code; got != http.StatusOK {
		t.Errorf("/healthz = %d, want 200", got)
	}
	if got := do(t, server, http.MethodGet, "/readyz", nil).Code; got != http.StatusServiceUnavailable {
		t.Errorf("/readyz = %d, want 503", got)
	}
}

func TestReadinessFollowsCheckResult(t *testing.T) {
	t.Parallel()

	first := &stubCheck{}
	second := &stubCheck{}
	server, _ := newServer(t, time.Second, []httpserver.ReadinessCheck{first, second})

	steps := []struct {
		name string
		set  error
		want int
	}{
		{"success", nil, http.StatusOK},
		{"failure", errors.New("dependency unavailable"), http.StatusServiceUnavailable},
		{"recovery", nil, http.StatusOK},
	}

	for _, step := range steps {
		first.fail(step.set)
		if got := do(t, server, http.MethodGet, "/readyz", nil).Code; got != step.want {
			t.Errorf("%s: /readyz = %d, want %d", step.name, got, step.want)
		}
		if got := do(t, server, http.MethodGet, "/healthz", nil).Code; got != http.StatusOK {
			t.Errorf("%s: /healthz = %d, want 200", step.name, got)
		}
	}
}

func TestReadinessTimeoutCancelsChecks(t *testing.T) {
	t.Parallel()

	cancelled := make(chan struct{})
	hung := checkFunc(func(ctx context.Context) error {
		<-ctx.Done()
		close(cancelled)
		return ctx.Err()
	})

	server, _ := newServer(t, 50*time.Millisecond, []httpserver.ReadinessCheck{hung})

	started := time.Now()
	recorder := do(t, server, http.MethodGet, "/readyz", nil)
	elapsed := time.Since(started)

	if recorder.Code != http.StatusServiceUnavailable {
		t.Errorf("/readyz = %d, want 503", recorder.Code)
	}
	if elapsed > 2*time.Second {
		t.Errorf("readiness took %v, want the configured budget", elapsed)
	}
	select {
	case <-cancelled:
	case <-time.After(2 * time.Second):
		t.Fatal("readiness check did not receive cancellation")
	}
}

func TestRequestIDSelection(t *testing.T) {
	t.Parallel()

	server, _ := newServer(t, time.Second, nil)
	maxLength := strings.Repeat("a", 64)

	cases := []struct {
		name   string
		values []string
		echo   string
	}{
		{name: "single character", values: []string{"a"}, echo: "a"},
		{name: "maximum length", values: []string{maxLength}, echo: maxLength},
		{name: "absent", values: nil},
		{name: "empty", values: []string{""}},
		{name: "too long", values: []string{strings.Repeat("a", 65)}},
		{name: "leading dot", values: []string{".abc"}},
		{name: "comma separated", values: []string{"a,b"}},
		{name: "duplicated", values: []string{"first", "second"}},
	}

	for _, tc := range cases {
		recorder := do(t, server, http.MethodGet, "/healthz", map[string][]string{"X-Request-Id": tc.values})
		if recorder.Code != http.StatusOK {
			t.Errorf("%s: status = %d, want 200", tc.name, recorder.Code)
		}
		got := recorder.Header().Get("X-Request-Id")
		if tc.echo != "" {
			if got != tc.echo {
				t.Errorf("%s: X-Request-Id = %q, want %q", tc.name, got, tc.echo)
			}
			continue
		}
		if !ulid.IsValid(got) {
			t.Errorf("%s: X-Request-Id = %q, want a generated ULID", tc.name, got)
		}
	}
}

func TestGeneratedRequestIDIsUnique(t *testing.T) {
	t.Parallel()

	server, _ := newServer(t, time.Second, nil)

	first := do(t, server, http.MethodGet, "/healthz", nil).Header().Get("X-Request-Id")
	second := do(t, server, http.MethodGet, "/healthz", nil).Header().Get("X-Request-Id")

	if !ulid.IsValid(first) || !ulid.IsValid(second) {
		t.Fatalf("generated identifiers are not ULIDs: %q, %q", first, second)
	}
	if first == second {
		t.Fatalf("generated identifiers repeat: %q", first)
	}
}

func TestFailureKeepsCorrelation(t *testing.T) {
	t.Parallel()

	failing := &stubCheck{err: errors.New("dependency unavailable")}
	server, _ := newServer(t, time.Second, []httpserver.ReadinessCheck{failing})

	recorder := do(t, server, http.MethodGet, "/readyz", map[string][]string{"X-Request-Id": {"0123456789abcdef0123456789abcdef"}})

	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("/readyz = %d, want 503", recorder.Code)
	}
	if got := recorder.Header().Get("X-Request-Id"); got != "0123456789abcdef0123456789abcdef" {
		t.Errorf("response X-Request-Id = %q", got)
	}
}

func TestLogRecordFields(t *testing.T) {
	t.Parallel()

	server, records := newServer(t, time.Second, nil)

	do(t, server, http.MethodGet, "/healthz", map[string][]string{"X-Request-Id": {"abcdef123456"}})
	do(t, server, http.MethodGet, "/readyz", nil)

	lines := decodeRecords(t, records.String())
	if len(lines) != 2 {
		t.Fatalf("records = %d, want 2", len(lines))
	}

	if lines[0]["request_id"] != "abcdef123456" {
		t.Errorf("request_id = %v", lines[0]["request_id"])
	}
	if lines[0]["service"] != "auth" {
		t.Errorf("service = %v", lines[0]["service"])
	}
	if lines[0]["operation"] != "health" {
		t.Errorf("operation = %v", lines[0]["operation"])
	}
	if lines[0]["result"] != "ok" {
		t.Errorf("result = %v", lines[0]["result"])
	}
	if _, ok := lines[0]["duration_ms"]; !ok {
		t.Error("duration_ms is missing")
	}

	if lines[1]["operation"] != "readiness" {
		t.Errorf("operation = %v", lines[1]["operation"])
	}
	if lines[1]["result"] != "error" {
		t.Errorf("result = %v", lines[1]["result"])
	}
}

func TestSecretsStayOutOfLog(t *testing.T) {
	t.Parallel()

	const secret = "database-password-marker"
	const token = "bearer-token-marker"

	failing := &stubCheck{err: errors.New("connection failed for " + secret)}
	server, records := newServer(t, time.Second, []httpserver.ReadinessCheck{failing})

	recorder := do(t, server, http.MethodGet, "/readyz", map[string][]string{
		"Authorization": {"Bearer " + token},
	})
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("/readyz = %d, want 503", recorder.Code)
	}

	logged := records.String()
	if strings.Contains(logged, secret) {
		t.Errorf("log contains the configuration secret: %s", logged)
	}
	if strings.Contains(logged, token) {
		t.Errorf("log contains the authorization token: %s", logged)
	}
}

func TestRequiredLogFieldsOnFailure(t *testing.T) {
	t.Parallel()

	failing := &stubCheck{err: errors.New("dependency unavailable")}
	server, records := newServer(t, time.Second, []httpserver.ReadinessCheck{failing})

	do(t, server, http.MethodGet, "/readyz", nil)

	lines := decodeRecords(t, records.String())
	if len(lines) != 1 {
		t.Fatalf("records = %d, want 1", len(lines))
	}
	for _, field := range []string{"request_id", "service", "operation", "result", "duration_ms"} {
		if _, ok := lines[0][field]; !ok {
			t.Errorf("field %s is missing in the failure record", field)
		}
	}
}

func decodeRecords(t *testing.T, raw string) []map[string]any {
	t.Helper()

	var records []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(raw), "\n") {
		if line == "" {
			continue
		}
		var record map[string]any
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatalf("decode record %q: %v", line, err)
		}
		records = append(records, record)
	}
	return records
}

type checkFunc func(context.Context) error

func (f checkFunc) Check(ctx context.Context) error {
	return f(ctx)
}
