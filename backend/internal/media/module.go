// Package media объявляет порты сообщений, вложений, подтверждений и обмена
// событиями и собирает модуль Media.
package media

import (
	"context"
	"io"
	"time"

	"github.com/MoloTeam/MoloTalk/backend/internal/platform/clock"
	"github.com/MoloTeam/MoloTalk/backend/internal/platform/httpserver"
	"github.com/MoloTeam/MoloTalk/backend/internal/platform/idgen"
)

// Message — сообщение чата.
type Message struct {
	ID      string
	ChatID  string
	Author  string
	Body    string
	Version int
}

// Attachment — вложение сообщения.
type Attachment struct {
	ID        string
	MessageID string
	Key       string
	Ready     bool
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

// ObjectInfo описывает объект файлового хранилища.
type ObjectInfo struct {
	Key  string
	Size int64
}

// MessageRepository хранит сообщения.
type MessageRepository interface {
	CreateOrGet(ctx context.Context, message Message) (Message, error)
	UpdateVersion(ctx context.Context, message Message) error
	SoftDelete(ctx context.Context, messageID string) error
	ListRange(ctx context.Context, chatID string, from, to int64) ([]Message, error)
}

// AttachmentRepository хранит вложения.
type AttachmentRepository interface {
	Get(ctx context.Context, attachmentID string) (Attachment, error)
	MarkReady(ctx context.Context, attachmentID string) error
	Link(ctx context.Context, attachmentID, messageID string) error
	CheckReferences(ctx context.Context, attachmentID string) error
}

// ReceiptRepository хранит подтверждения прочтения.
type ReceiptRepository interface {
	UpsertMonotonic(ctx context.Context, receipt Receipt) error
	GetSummary(ctx context.Context, messageID string) (ReceiptSummary, error)
	GetReadStates(ctx context.Context, chatID string) ([]ReadState, error)
}

// ObjectStorage хранит вложения в файловом хранилище.
type ObjectStorage interface {
	PresignPut(ctx context.Context, key string, ttl time.Duration) (string, error)
	PresignGet(ctx context.Context, key string, ttl time.Duration) (string, error)
	Open(ctx context.Context, key string) (io.ReadCloser, error)
	Finalize(ctx context.Context, key string) error
	Head(ctx context.Context, key string) (ObjectInfo, error)
	Delete(ctx context.Context, key string) error
}

// EventPublisher публикует события обмена.
type EventPublisher interface {
	Publish(ctx context.Context, topic string, payload []byte) error
}

// EventSubscriber подписывается на события обмена.
type EventSubscriber interface {
	Subscribe(ctx context.Context, topic string, handler func(context.Context, []byte) error) error
}

// Deps задаёт зависимости модуля Media.
type Deps struct {
	Messages    MessageRepository
	Attachments AttachmentRepository
	Receipts    ReceiptRepository
	Objects     ObjectStorage
	Publisher   EventPublisher
	Subscriber  EventSubscriber
	Clock       clock.Clock
	IDs         idgen.IDGenerator
}

// Module — прикладной модуль Media.
type Module struct {
	deps Deps
}

// New собирает модуль с переданными зависимостями.
func New(deps Deps) *Module {
	return &Module{deps: deps}
}

// Name возвращает имя процесса.
func (m *Module) Name() string {
	return "media"
}

// Readiness возвращает проверки готовности модуля. В каркасе T-01 набор пуст.
func (m *Module) Readiness() []httpserver.ReadinessCheck {
	return nil
}
