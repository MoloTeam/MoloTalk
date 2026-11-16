// Package clock предоставляет текущее время и управляемую тестовую реализацию.
package clock

import (
	"sync"
	"time"
)

// Clock отдаёт текущее время.
type Clock interface {
	Now() time.Time
}

// System читает системные часы.
type System struct{}

// Now возвращает текущее системное время.
func (System) Now() time.Time {
	return time.Now()
}

// Manual — управляемая шкала времени для тестов.
type Manual struct {
	mu  sync.Mutex
	now time.Time
}

// NewManual создаёт часы, установленные на заданный момент.
func NewManual(start time.Time) *Manual {
	return &Manual{now: start}
}

// Now возвращает текущее значение управляемой шкалы.
func (m *Manual) Now() time.Time {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.now
}

// Advance продвигает шкалу на заданную длительность.
func (m *Manual) Advance(delta time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.now = m.now.Add(delta)
}

// Set устанавливает шкалу на заданный момент.
func (m *Manual) Set(moment time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.now = moment
}
