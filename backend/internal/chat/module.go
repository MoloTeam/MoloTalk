// Package chat объявляет порты состава чата, сообщений, подтверждений и
// транзакций и собирает модуль Chat.
package chat

import (
	"context"
	"time"

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

// Message — сообщение чата.
type Message struct {
	ID      string
	ChatID  string
	Author  string
	Body    string
	Version int
}

// Receipt — подтверждение прочтения.
type Receipt struct {
	MessageID string
	AccountID string
	ReadAt    time.Time
}

// ReceiptSummary — сводка подтверждений сообщения.
type ReceiptSummary struct {
	Read  int
	Total int
}

// ReadState — состояние прочтения участника.
type ReadState struct {
	AccountID string
	MessageID string
}

// ChatRepository хранит состав чата.
type ChatRepository interface {
	GetMembers(ctx context.Context, chatID string) ([]Member, error)
	LockGroup(ctx context.Context, chatID string) error
	AddMember(ctx context.Context, chatID, accountID string) error
	ChangeOwner(ctx context.Context, chatID, accountID string) error
}

// MessageRepository хранит сообщения.
type MessageRepository interface {
	CreateOrGet(ctx context.Context, message Message) (Message, error)
	UpdateVersion(ctx context.Context, message Message) error
	SoftDelete(ctx context.Context, messageID string) error
	ListRange(ctx context.Context, chatID string, from, to int64) ([]Message, error)
}

// ReceiptRepository хранит подтверждения прочтения.
type ReceiptRepository interface {
	UpsertMonotonic(ctx context.Context, receipt Receipt) error
	GetSummary(ctx context.Context, messageID string) (ReceiptSummary, error)
	GetReadStates(ctx context.Context, chatID string) ([]ReadState, error)
}

// GroupUnitOfWork выполняет группу операций в одной транзакции.
type GroupUnitOfWork interface {
	WithinTransaction(ctx context.Context, fn func(ctx context.Context) error) error
}

// Deps задаёт зависимости модуля Chat.
type Deps struct {
	Chats      ChatRepository
	Messages   MessageRepository
	Receipts   ReceiptRepository
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
