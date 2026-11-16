package logging_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/MoloTeam/MoloTalk/backend/internal/platform/logging"
)

const requestID = "01ARZ3NDEKTSV4RRFFQ69G5FAV"

func TestRecordContainsRequiredFields(t *testing.T) {
	t.Parallel()

	output := &bytes.Buffer{}
	logger := logging.New(output, "media")

	logger.Write(logging.Record{
		RequestID: requestID,
		Service:   "media",
		Operation: "send",
		Result:    logging.ResultOK,
		Duration:  42 * time.Millisecond,
	})

	var record map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(output.Bytes()), &record); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if record["request_id"] != requestID {
		t.Errorf("request_id = %v, want %v", record["request_id"], requestID)
	}
	if record["service"] != "media" {
		t.Errorf("service = %v", record["service"])
	}
	if record["operation"] != "send" {
		t.Errorf("operation = %v", record["operation"])
	}
	if record["result"] != "ok" {
		t.Errorf("result = %v", record["result"])
	}
	if record["duration_ms"] != float64(42) {
		t.Errorf("duration_ms = %v, want 42", record["duration_ms"])
	}
}

func TestIdentifiersAppearOnlyWhenPresent(t *testing.T) {
	t.Parallel()

	output := &bytes.Buffer{}
	logger := logging.New(output, "chat")
	logger.Write(logging.Record{RequestID: requestID, Result: logging.ResultOK})

	var plain map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(output.Bytes()), &plain); err != nil {
		t.Fatalf("decode: %v", err)
	}
	for _, field := range []string{"user_id", "chat_id", "message_id"} {
		if _, ok := plain[field]; ok {
			t.Errorf("field %s must be omitted when empty", field)
		}
	}

	output.Reset()
	logger.Write(logging.Record{RequestID: requestID, Result: logging.ResultOK, UserID: "u-1", ChatID: "c-1", MessageID: "m-1"})

	var full map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(output.Bytes()), &full); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if full["user_id"] != "u-1" || full["chat_id"] != "c-1" || full["message_id"] != "m-1" {
		t.Errorf("identifiers are missing: %v", full)
	}
}

func TestRecordNeverCarriesSecrets(t *testing.T) {
	t.Parallel()

	output := &bytes.Buffer{}
	logging.New(output, "auth").Write(logging.Record{
		RequestID: requestID,
		Operation: "login",
		Result:    logging.ResultError,
	})

	logged := strings.ToLower(output.String())
	for _, value := range []string{"password", "token", "authorization", "secret"} {
		if strings.Contains(logged, value) {
			t.Errorf("log contains %q: %s", value, output.String())
		}
	}
}

func TestNilLoggerIsSafe(t *testing.T) {
	t.Parallel()

	var logger *logging.Logger
	logger.Write(logging.Record{})
}
