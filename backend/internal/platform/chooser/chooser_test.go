package chooser_test

import (
	"errors"
	"testing"

	"github.com/MoloTeam/MoloTalk/backend/internal/platform/chooser"
)

func TestSingleMemberIsChosen(t *testing.T) {
	t.Parallel()

	got, err := chooser.Random{}.Choose([]string{"alice"})
	if err != nil {
		t.Fatalf("choose: %v", err)
	}
	if got != "alice" {
		t.Fatalf("got %q, want alice", got)
	}
}

func TestRandomChoiceStaysInsideTheAudience(t *testing.T) {
	t.Parallel()

	members := []string{"alice", "bob", "carol"}
	for range 20 {
		got, err := chooser.Random{}.Choose(members)
		if err != nil {
			t.Fatalf("choose: %v", err)
		}
		found := false
		for _, member := range members {
			if member == got {
				found = true
			}
		}
		if !found {
			t.Fatalf("chosen %q is outside the audience", got)
		}
	}
}

func TestFixedCandidates(t *testing.T) {
	t.Parallel()

	members := []string{"alice", "bob", "carol"}

	got, err := chooser.Fixed{Candidate: "bob"}.Choose(members)
	if err != nil {
		t.Fatalf("choose: %v", err)
	}
	if got != "bob" {
		t.Fatalf("got %q, want bob", got)
	}
}

func TestChoiceIsImpossible(t *testing.T) {
	t.Parallel()

	if got, err := (chooser.Random{}).Choose(nil); !errors.Is(err, chooser.ErrNoMembers) || got != "" {
		t.Fatalf("empty audience: got %q, %v", got, err)
	}
	if got, err := (chooser.Fixed{Candidate: "bob"}).Choose([]string{"alice", "carol"}); !errors.Is(err, chooser.ErrCandidateNotInList) || got != "" {
		t.Fatalf("candidate outside: got %q, %v", got, err)
	}
}
