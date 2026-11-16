package clock_test

import (
	"testing"
	"time"

	"github.com/MoloTeam/MoloTalk/backend/internal/platform/clock"
)

func TestManualTimeAdvancesOnlyOnCommand(t *testing.T) {
	t.Parallel()

	start := time.Date(2026, time.October, 19, 9, 0, 0, 0, time.UTC)
	manual := clock.NewManual(start)

	if first := manual.Now(); !first.Equal(start) {
		t.Fatalf("first = %v, want %v", first, start)
	}
	if second := manual.Now(); !second.Equal(start) {
		t.Fatalf("second = %v, want %v", second, start)
	}

	manual.Advance(time.Minute)

	want := start.Add(time.Minute)
	if third := manual.Now(); !third.Equal(want) {
		t.Fatalf("third = %v, want %v", third, want)
	}
}

func TestSetReplacesTheScale(t *testing.T) {
	t.Parallel()

	manual := clock.NewManual(time.Date(2026, time.October, 19, 9, 0, 0, 0, time.UTC))
	moment := time.Date(2027, time.February, 1, 12, 30, 0, 0, time.UTC)
	manual.Set(moment)

	if got := manual.Now(); !got.Equal(moment) {
		t.Fatalf("now = %v, want %v", got, moment)
	}
}

func TestSystemClockMovesForward(t *testing.T) {
	t.Parallel()

	before := clock.System{}.Now()
	time.Sleep(time.Millisecond)
	after := clock.System{}.Now()

	if !after.After(before) {
		t.Fatalf("system clock did not move: %v then %v", before, after)
	}
}
