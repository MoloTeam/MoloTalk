package media_test

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/MoloTeam/MoloTalk/backend/internal/media"
	"github.com/MoloTeam/MoloTalk/backend/internal/platform/clock"
	"github.com/MoloTeam/MoloTalk/backend/internal/platform/idgen"
)

type messages struct{ err error }

func (f messages) CreateOrGet(context.Context, media.Message) (media.Message, error) {
	return media.Message{}, f.err
}

func (f messages) UpdateVersion(context.Context, media.Message) error { return f.err }

func (f messages) SoftDelete(context.Context, string) error { return f.err }

func (f messages) ListRange(context.Context, string, int64, int64) ([]media.Message, error) {
	return nil, f.err
}

type attachments struct{ err error }

func (f attachments) Get(context.Context, string) (media.Attachment, error) {
	return media.Attachment{}, f.err
}

func (f attachments) MarkReady(context.Context, string) error { return f.err }

func (f attachments) Link(context.Context, string, string) error { return f.err }

func (f attachments) CheckReferences(context.Context, string) error { return f.err }

type receipts struct{ err error }

func (f receipts) UpsertMonotonic(context.Context, media.Receipt) error { return f.err }

func (f receipts) GetSummary(context.Context, string) (media.ReceiptSummary, error) {
	return media.ReceiptSummary{}, f.err
}

func (f receipts) GetReadStates(context.Context, string) ([]media.ReadState, error) {
	return nil, f.err
}

type objects struct{ err error }

func (f objects) PresignPut(context.Context, string, time.Duration) (string, error) {
	return "", f.err
}

func (f objects) PresignGet(context.Context, string, time.Duration) (string, error) {
	return "", f.err
}

func (f objects) Open(context.Context, string) (io.ReadCloser, error) { return nil, f.err }

func (f objects) Finalize(context.Context, string) error { return f.err }

func (f objects) Head(context.Context, string) (media.ObjectInfo, error) {
	return media.ObjectInfo{}, f.err
}

func (f objects) Delete(context.Context, string) error { return f.err }

type publisher struct{ err error }

func (f publisher) Publish(context.Context, string, []byte) error { return f.err }

type subscriber struct{ err error }

func (f subscriber) Subscribe(context.Context, string, func(context.Context, []byte) error) error {
	return f.err
}

var (
	_ media.MessageRepository    = messages{}
	_ media.AttachmentRepository = attachments{}
	_ media.ReceiptRepository    = receipts{}
	_ media.ObjectStorage        = objects{}
	_ media.EventPublisher       = publisher{}
	_ media.EventSubscriber      = subscriber{}
)

func TestMediaPortsReproduceFailuresInIsolation(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("storage unavailable")
	module := media.New(media.Deps{
		Messages:    messages{err: sentinel},
		Attachments: attachments{err: sentinel},
		Receipts:    receipts{err: sentinel},
		Objects:     objects{err: sentinel},
		Publisher:   publisher{err: sentinel},
		Subscriber:  subscriber{err: sentinel},
		Clock:       clock.NewManual(time.Unix(0, 0)),
		IDs:         idgen.NewSequence(),
	})

	if module == nil || module.Name() != "media" {
		t.Fatal("module was not constructed")
	}

	ctx := context.Background()

	var attachmentsPort media.AttachmentRepository = attachments{err: sentinel}
	if err := attachmentsPort.Link(ctx, "attachment", "message"); !errors.Is(err, sentinel) {
		t.Errorf("Link error = %v, want the port failure", err)
	}

	var objectsPort media.ObjectStorage = objects{err: sentinel}
	if _, err := objectsPort.Head(ctx, "key"); !errors.Is(err, sentinel) {
		t.Errorf("Head error = %v, want the port failure", err)
	}

	var publisherPort media.EventPublisher = publisher{err: sentinel}
	if err := publisherPort.Publish(ctx, "topic", nil); !errors.Is(err, sentinel) {
		t.Errorf("Publish error = %v, want the port failure", err)
	}
}
