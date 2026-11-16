package ulid_test

import (
	"strings"
	"testing"

	"github.com/MoloTeam/MoloTalk/backend/internal/platform/ulid"
)

func TestNewProducesValidIdentifier(t *testing.T) {
	t.Parallel()

	id, err := ulid.New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	if len(id.String()) != ulid.Length {
		t.Fatalf("length = %d, want %d", len(id.String()), ulid.Length)
	}
	if !ulid.IsValid(id.String()) {
		t.Fatalf("%q is not a valid ulid", id.String())
	}
}

func TestNewIsRandom(t *testing.T) {
	t.Parallel()

	first, err := ulid.New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	second, err := ulid.New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	if first.String() == second.String() {
		t.Fatal("two identifiers are equal")
	}
}

func TestParseRoundTrip(t *testing.T) {
	t.Parallel()

	id, err := ulid.New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}

	parsed, err := ulid.Parse(id.String())
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if parsed != id {
		t.Fatalf("round trip differs: %v != %v", parsed, id)
	}
}

func TestParseRejectsInvalidValues(t *testing.T) {
	t.Parallel()

	for _, value := range []string{"", "short", strings.Repeat("Z", 25), strings.Repeat("I", 26)} {
		if _, err := ulid.Parse(value); err == nil {
			t.Errorf("%q: expected error", value)
		}
	}
}
