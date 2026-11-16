package chat_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/MoloTeam/MoloTalk/backend/internal/chat"
	"github.com/MoloTeam/MoloTalk/backend/internal/platform/chooser"
	"github.com/MoloTeam/MoloTalk/backend/internal/platform/clock"
	"github.com/MoloTeam/MoloTalk/backend/internal/platform/idgen"
)

type chats struct{ err error }

func (f chats) GetMembers(context.Context, string) ([]chat.Member, error) {
	return nil, f.err
}

func (f chats) LockGroup(context.Context, string) error { return f.err }

func (f chats) AddMember(context.Context, string, string) error { return f.err }

func (f chats) ChangeOwner(context.Context, string, string) error { return f.err }

type messages struct{ err error }

func (f messages) CreateOrGet(context.Context, chat.Message) (chat.Message, error) {
	return chat.Message{}, f.err
}

func (f messages) UpdateVersion(context.Context, chat.Message) error { return f.err }

func (f messages) SoftDelete(context.Context, string) error { return f.err }

func (f messages) ListRange(context.Context, string, int64, int64) ([]chat.Message, error) {
	return nil, f.err
}

type receipts struct{ err error }

func (f receipts) UpsertMonotonic(context.Context, chat.Receipt) error { return f.err }

func (f receipts) GetSummary(context.Context, string) (chat.ReceiptSummary, error) {
	return chat.ReceiptSummary{}, f.err
}

func (f receipts) GetReadStates(context.Context, string) ([]chat.ReadState, error) {
	return nil, f.err
}

type unitOfWork struct{ err error }

func (f unitOfWork) WithinTransaction(ctx context.Context, fn func(context.Context) error) error {
	if f.err != nil {
		return f.err
	}
	return fn(ctx)
}

var (
	_ chat.ChatRepository    = chats{}
	_ chat.MessageRepository = messages{}
	_ chat.ReceiptRepository = receipts{}
	_ chat.GroupUnitOfWork   = unitOfWork{}
)

func TestChatPortsSeparateCompositionAndTransaction(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("chat storage unavailable")
	module := chat.New(chat.Deps{
		Chats:      chats{err: sentinel},
		Messages:   messages{err: sentinel},
		Receipts:   receipts{err: sentinel},
		UnitOfWork: unitOfWork{err: sentinel},
		Chooser:    chooser.Fixed{Candidate: "bob"},
		Clock:      clock.NewManual(time.Unix(0, 0)),
		IDs:        idgen.NewSequence(),
	})

	if module == nil || module.Name() != "chat" {
		t.Fatal("module was not constructed")
	}

	ctx := context.Background()
	var repository chat.ChatRepository = chats{err: sentinel}
	if _, err := repository.GetMembers(ctx, "chat"); !errors.Is(err, sentinel) {
		t.Errorf("GetMembers error = %v, want the port failure", err)
	}

	var transaction chat.GroupUnitOfWork = unitOfWork{err: sentinel}
	if err := transaction.WithinTransaction(ctx, func(context.Context) error { return nil }); !errors.Is(err, sentinel) {
		t.Errorf("WithinTransaction error = %v, want the port failure", err)
	}
}

func TestChatOwnsMessageAndReceiptPorts(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("message storage unavailable")
	ctx := context.Background()

	var messagePort chat.MessageRepository = messages{err: sentinel}
	if err := messagePort.SoftDelete(ctx, "message"); !errors.Is(err, sentinel) {
		t.Errorf("SoftDelete error = %v, want the port failure", err)
	}

	var receiptPort chat.ReceiptRepository = receipts{err: sentinel}
	if _, err := receiptPort.GetSummary(ctx, "message"); !errors.Is(err, sentinel) {
		t.Errorf("GetSummary error = %v, want the port failure", err)
	}
}
