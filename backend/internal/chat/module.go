// Package chat объявляет порты состава чата и транзакций и собирает модуль Chat.
package chat

import (
	"context"

	"github.com/MoloTeam/MoloTalk/backend/internal/platform/chooser"
	"github.com/MoloTeam/MoloTalk/backend/internal/platform/clock"
	"github.com/MoloTeam/MoloTalk/backend/internal/platform/httpserver"
	"github.com/MoloTeam/MoloTalk/backend/internal/platform/idgen"
)

// Member — участник чата.
type Member struct {
	ID          string
	DisplayName string
}

// ChatRepository хранит состав чата.
type ChatRepository interface {
	GetMembers(ctx context.Context, chatID string) ([]Member, error)
	LockGroup(ctx context.Context, chatID string) error
	AddMember(ctx context.Context, chatID, accountID string) error
	ChangeOwner(ctx context.Context, chatID, accountID string) error
}

// GroupUnitOfWork выполняет группу операций в одной транзакции.
type GroupUnitOfWork interface {
	WithinTransaction(ctx context.Context, fn func(ctx context.Context) error) error
}

// Deps задаёт зависимости модуля Chat.
type Deps struct {
	Chats      ChatRepository
	UnitOfWork GroupUnitOfWork
	Chooser    chooser.MemberChooser
	Clock      clock.Clock
	IDs        idgen.IDGenerator
}

// Module — прикладной модуль Chat.
type Module struct {
	deps Deps
}

// New собирает модуль с переданными зависимостями.
func New(deps Deps) *Module {
	return &Module{deps: deps}
}

// Name возвращает имя процесса.
func (m *Module) Name() string {
	return "chat"
}

// Readiness возвращает проверки готовности модуля. В каркасе T-01 набор пуст.
func (m *Module) Readiness() []httpserver.ReadinessCheck {
	return nil
}
