package idgen_test

import (
	"encoding/binary"
	"errors"
	"regexp"
	"sync"
	"testing"
	"time"

	"github.com/oklog/ulid/v2"

	"github.com/MoloTeam/MoloTalk/backend/internal/platform/clock"
	"github.com/MoloTeam/MoloTalk/backend/internal/platform/idgen"
)

// ulidDomain повторяет ограничение столбца ULID из schema.sql.
var ulidDomain = regexp.MustCompile(`^[0-7][0-9A-HJKMNP-TV-Z]{25}$`)

func mustParse(t *testing.T, value string) ulid.ULID {
	t.Helper()

	id, err := ulid.ParseStrict(value)
	if err != nil {
		t.Fatalf("parse %q: %v", value, err)
	}
	return id
}

func TestSystemTakesTimeFromClock(t *testing.T) {
	t.Parallel()

	moment := time.Date(2026, time.November, 16, 12, 30, 0, 0, time.UTC)
	generator := idgen.NewSystem(clock.NewManual(moment))

	id, err := generator.NewID()
	if err != nil {
		t.Fatalf("new id: %v", err)
	}

	want := uint64(moment.UnixMilli())
	if got := id.Time(); got != want {
		t.Fatalf("ulid.Time() = %d, want %d", got, want)
	}

	// Первые 48 бит идентификатора — это Unix-миллисекунды.
	first48 := binary.BigEndian.Uint64(id[0:8]) >> 16
	if first48 != want {
		t.Fatalf("first 48 bits = %d, want %d", first48, want)
	}
}

func TestSystemIsStrictlyIncreasingWithinOneMillisecond(t *testing.T) {
	t.Parallel()

	generator := idgen.NewSystem(clock.NewManual(time.Date(2026, time.November, 16, 12, 30, 0, 0, time.UTC)))

	previous := ulid.ULID{}
	for index := range 1000 {
		current, err := generator.NewID()
		if err != nil {
			t.Fatalf("id %d: %v", index, err)
		}
		if index > 0 && current.String() <= previous.String() {
			t.Fatalf("id %d (%s) is not greater than %s", index, current, previous)
		}
		previous = current
	}
}

func TestSystemMatchesDatabaseDomain(t *testing.T) {
	t.Parallel()

	generator := idgen.NewSystem(clock.System{})

	for index := range 100 {
		id, err := generator.NewID()
		if err != nil {
			t.Fatalf("id %d: %v", index, err)
		}
		if !ulidDomain.MatchString(id.String()) {
			t.Fatalf("id %q does not match the schema.sql domain", id.String())
		}
	}
}

func TestSystemIsSafeForConcurrentUse(t *testing.T) {
	t.Parallel()

	const (
		workers   = 8
		perWorker = 100
	)

	generator := idgen.NewSystem(clock.NewManual(time.Date(2026, time.November, 16, 12, 30, 0, 0, time.UTC)))

	var (
		mu   sync.Mutex
		seen = make(map[string]bool, workers*perWorker)
		wg   sync.WaitGroup
	)
	wg.Add(workers)
	for range workers {
		go func() {
			defer wg.Done()
			for range perWorker {
				id, err := generator.NewID()
				if err != nil {
					t.Errorf("new id: %v", err)
					return
				}
				mu.Lock()
				if seen[id.String()] {
					t.Errorf("identifier repeats: %s", id)
				}
				seen[id.String()] = true
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	if len(seen) != workers*perWorker {
		t.Fatalf("identifiers = %d, want %d", len(seen), workers*perWorker)
	}
}

func TestSequenceReturnsGivenIdentifiersInOrder(t *testing.T) {
	t.Parallel()

	first := mustParse(t, "01ARZ3NDEKTSV4RRFFQ69G5FAV")
	second := mustParse(t, "01BX5ZZKBKACTAV9WEVGEMMVRZ")
	sequence := idgen.NewSequence(first, second)

	for index, want := range []ulid.ULID{first, second} {
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

	sequence := idgen.NewSequence(mustParse(t, "01ARZ3NDEKTSV4RRFFQ69G5FAV"))
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
