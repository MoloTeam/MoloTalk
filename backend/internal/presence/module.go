// Package presence объявляет порты состояния присутствия и межсервисных
// клиентов и собирает модуль Presence.
package presence

import (
	"context"
	"time"

	"github.com/MoloTeam/MoloTalk/backend/internal/platform/clock"
	"github.com/MoloTeam/MoloTalk/backend/internal/platform/httpserver"
	"github.com/MoloTeam/MoloTalk/backend/internal/platform/idgen"
)

// Presence — состояние присутствия участника.
type Presence struct {
	AccountID string
	ChatID    string
	Until     time.Time
}

// PresenceStateStore хранит состояние присутствия.
type PresenceStateStore interface {
	Renew(ctx context.Context, presence Presence) error
	Remove(ctx context.Context, accountID string) error
	ListLive(ctx context.Context, chatID string) ([]Presence, error)
}

// AuthClient проверяет сеанс в сервисе Auth.
type AuthClient interface {
	CheckSession(ctx context.Context, sessionID string) (bool, error)
}

// ChatAccessClient проверяет доступ к чату в сервисе Chat.
type ChatAccessClient interface {
	CheckAccess(ctx context.Context, chatID, accountID string) (bool, error)
	GetAudience(ctx context.Context, chatID string) ([]string, error)
}

// Deps задаёт зависимости модуля Presence.
type Deps struct {
	States PresenceStateStore
	Auth   AuthClient
	Chats  ChatAccessClient
	Clock  clock.Clock
	IDs    idgen.IDGenerator
}

// Module — прикладной модуль Presence.
type Module struct {
	deps Deps
}

// New собирает модуль с переданными зависимостями.
func New(deps Deps) *Module {
	return &Module{deps: deps}
}

// Name возвращает имя процесса.
func (m *Module) Name() string {
	return "presence"
}

// Readiness возвращает проверки готовности модуля. В каркасе T-01 набор пуст.
func (m *Module) Readiness() []httpserver.ReadinessCheck {
	return nil
}
