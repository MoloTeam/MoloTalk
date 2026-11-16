// Package gateway объявляет межсервисные порты и собирает модуль Gateway.
package gateway

import (
	"context"

	"github.com/MoloTeam/MoloTalk/backend/internal/platform/clock"
	"github.com/MoloTeam/MoloTalk/backend/internal/platform/httpserver"
	"github.com/MoloTeam/MoloTalk/backend/internal/platform/idgen"
)

// Session — проверенный сеанс, полученный от сервиса Auth.
type Session struct {
	ID        string
	AccountID string
}

// AuthClient проверяет сеанс в сервисе Auth.
type AuthClient interface {
	CheckSession(ctx context.Context, sessionID string) (Session, error)
}

// ChatAccessClient проверяет доступ к чату в сервисе Chat.
type ChatAccessClient interface {
	CheckAccess(ctx context.Context, chatID, accountID string) (bool, error)
	GetAudience(ctx context.Context, chatID string) ([]string, error)
}

// EventSubscriber подписывается на события обмена.
type EventSubscriber interface {
	Subscribe(ctx context.Context, topic string, handler func(context.Context, []byte) error) error
}

// Deps задаёт зависимости модуля Gateway.
type Deps struct {
	Auth       AuthClient
	Chats      ChatAccessClient
	Subscriber EventSubscriber
	Clock      clock.Clock
	IDs        idgen.IDGenerator
}

// Module — прикладной модуль Gateway.
type Module struct {
	deps Deps
}

// New собирает модуль с переданными зависимостями.
func New(deps Deps) *Module {
	return &Module{deps: deps}
}

// Name возвращает имя процесса.
func (m *Module) Name() string {
	return "gateway"
}

// Readiness возвращает проверки готовности модуля. В каркасе T-01 набор пуст.
func (m *Module) Readiness() []httpserver.ReadinessCheck {
	return nil
}
