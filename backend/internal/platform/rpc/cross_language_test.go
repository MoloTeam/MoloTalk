package rpc_test

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"

	molotalkv1 "github.com/MoloTeam/MoloTalk/backend/gen/molotalk/v1"
)

// Кадр ServerEvent собран TypeScript-тестом и закоммичен как эталон.
func TestGoDecodesTypeScriptFrame(t *testing.T) {
	t.Parallel()

	encoded, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "api", "testdata", "gateway-server-event.b64"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	payload, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(encoded)))
	if err != nil {
		t.Fatalf("decode base64: %v", err)
	}

	event := &molotalkv1.ServerEvent{}
	if err := proto.Unmarshal(payload, event); err != nil {
		t.Fatalf("unmarshal event: %v", err)
	}

	if event.GetEventId() != "01ARZ3NDEKTSV4RRFFQ69G5FA1" {
		t.Fatalf("event id = %q", event.GetEventId())
	}
	if event.GetOccurredAt().GetSeconds() != 1795000000 {
		t.Fatalf("occurred at = %v", event.GetOccurredAt())
	}

	deleted := event.GetMessageDeleted()
	if deleted == nil {
		t.Fatalf("event case = %T, want message deleted", event.Event)
	}
	if deleted.GetChatId() != "01ARZ3NDEKTSV4RRFFQ69G5FA2" || deleted.GetMessageId() != "01ARZ3NDEKTSV4RRFFQ69G5FA3" {
		t.Fatalf("deleted ids = %q/%q", deleted.GetChatId(), deleted.GetMessageId())
	}
	if deleted.GetRevision() != 2 {
		t.Fatalf("revision = %d", deleted.GetRevision())
	}
}
