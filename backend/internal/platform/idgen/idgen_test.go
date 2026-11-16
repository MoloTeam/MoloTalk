package idgen_test

import (
	"errors"
	"testing"

	"github.com/MoloTeam/MoloTalk/backend/internal/platform/idgen"
	"github.com/MoloTeam/MoloTalk/backend/internal/platform/ulid"
)

func mustULID(t *testing.T, value string) ulid.ID {
	t.Helper()

	id, err := ulid.Parse(value)
	if err != nil {
		t.Fatalf("parse %q: %v", value, err)
	}
	return id
}

func TestSequenceReturnsGivenIdentifiersInOrder(t *testing.T) {
	t.Parallel()

	first := mustULID(t, "01ARZ3NDEKTSV4RRFFQ69G5FAV")
	second := mustULID(t, "01BX5ZZKBKACTAV9WEVGEMMVRZ")
	sequence := idgen.NewSequence(first, second)

	for index, want := range []ulid.ID{first, second} {
		got, err := sequence.NewID()
		if err != nil {
			t.Fatalf("call %d: %v", index+1, err)
		}
		if got != want {
			t.Fatalf("call %d = %v, want %v", index+1, got, want)
		}
	}

	if _, err := sequence.NewID(); !errors.Is(err, idgen.ErrExhausted) {
		t.Fatalf("third call error = %v, want ErrExhausted", err)
	}
}

func TestSequenceReportsRemaining(t *testing.T) {
	t.Parallel()

	sequence := idgen.NewSequence(mustULID(t, "01ARZ3NDEKTSV4RRFFQ69G5FAV"))
	if got := sequence.Remaining(); got != 1 {
		t.Fatalf("remaining = %d, want 1", got)
	}
	if _, err := sequence.NewID(); err != nil {
		t.Fatalf("new id: %v", err)
	}
	if got := sequence.Remaining(); got != 0 {
		t.Fatalf("remaining = %d, want 0", got)
	}
}

func TestSystemGeneratorProducesValidIdentifiers(t *testing.T) {
	t.Parallel()

	generator := idgen.System{}
	seen := make(map[string]bool)

	for range 3 {
		id, err := generator.NewID()
		if err != nil {
			t.Fatalf("new id: %v", err)
		}
		if !ulid.IsValid(id.String()) {
			t.Fatalf("%q is not a valid ulid", id.String())
		}
		if seen[id.String()] {
			t.Fatalf("identifier repeats: %q", id.String())
		}
		seen[id.String()] = true
	}
}
