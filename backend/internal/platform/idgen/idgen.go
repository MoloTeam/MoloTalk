// Package idgen предоставляет генераторы идентификаторов и их тестовую очередь.
package idgen

import (
	"errors"
	"sync"

	"github.com/MoloTeam/MoloTalk/backend/internal/platform/ulid"
)

// IDGenerator выдаёт идентификаторы операций.
type IDGenerator interface {
	NewID() (ulid.ID, error)
}

// ErrExhausted возвращается, когда заданная последовательность исчерпана.
var ErrExhausted = errors.New("id sequence exhausted")

// System создаёт ULID из криптографически стойкого источника случайности.
type System struct{}

// NewID возвращает новый ULID.
func (System) NewID() (ulid.ID, error) {
	return ulid.New()
}

// Sequence выдаёт заранее заданные идентификаторы по порядку.
type Sequence struct {
	mu    sync.Mutex
	ids   []ulid.ID
	index int
}

// NewSequence создаёт очередь из заданных идентификаторов.
func NewSequence(ids ...ulid.ID) *Sequence {
	return &Sequence{ids: ids}
}

// NewID возвращает следующий идентификатор или ErrExhausted.
func (s *Sequence) NewID() (ulid.ID, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.index >= len(s.ids) {
		return ulid.ID{}, ErrExhausted
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
