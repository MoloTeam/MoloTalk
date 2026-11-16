// Package idgen предоставляет генераторы идентификаторов и их тестовую очередь.
package idgen

import (
	"crypto/rand"
	"errors"
	"io"
	"sync"

	"github.com/oklog/ulid/v2"

	"github.com/MoloTeam/MoloTalk/backend/internal/platform/clock"
)

// IDGenerator выдаёт идентификаторы операций.
type IDGenerator interface {
	NewID() (ulid.ULID, error)
}

// ErrExhausted возвращается, когда заданная последовательность исчерпана.
var ErrExhausted = errors.New("id sequence exhausted")

// System создаёт ULID из времени управляемых часов и криптографически стойкой
// энтропии, монотонной в пределах одной миллисекунды.
type System struct {
	clock   clock.Clock
	entropy io.Reader
}

// NewSystem создаёт генератор, берущий время из переданных часов.
func NewSystem(source clock.Clock) *System {
	if source == nil {
		source = clock.System{}
	}
	return &System{
		clock: source,
		// LockedMonotonicReader — эквивалент LockedMonotonicEntropy из
		// oklog/ulid: монотонная энтропия под мьютексом, безопасная для
		// параллельных вызовов.
		entropy: &ulid.LockedMonotonicReader{MonotonicReader: ulid.Monotonic(rand.Reader, 0)},
	}
}

// NewID возвращает ULID на момент Clock.Now().
func (s *System) NewID() (ulid.ULID, error) {
	return ulid.New(ulid.Timestamp(s.clock.Now()), s.entropy)
}

// Sequence выдаёт заранее заданные идентификаторы по порядку.
type Sequence struct {
	mu    sync.Mutex
	ids   []ulid.ULID
	index int
}

// NewSequence создаёт очередь из заданных идентификаторов.
func NewSequence(ids ...ulid.ULID) *Sequence {
	return &Sequence{ids: ids}
}

// NewID возвращает следующий идентификатор или ErrExhausted.
func (s *Sequence) NewID() (ulid.ULID, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.index >= len(s.ids) {
		return ulid.ULID{}, ErrExhausted
	}
	id := s.ids[s.index]
	s.index++
	return id, nil
}

// Remaining сообщает, сколько идентификаторов осталось в очереди.
func (s *Sequence) Remaining() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.ids) - s.index
}
