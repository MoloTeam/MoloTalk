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

type attachments struct{ err error }

func (f attachments) Get(context.Context, string) (media.Attachment, error) {
	return media.Attachment{}, f.err
}

func (f attachments) MarkReady(context.Context, string) error { return f.err }

func (f attachments) Link(context.Context, string, string) error { return f.err }

func (f attachments) CheckReferences(context.Context, string) error { return f.err }

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
	_ media.AttachmentRepository = attachments{}
	_ media.ObjectStorage        = objects{}
	_ media.EventPublisher       = publisher{}
	_ media.EventSubscriber      = subscriber{}
)

func TestMediaPortsReproduceFailuresInIsolation(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("storage unavailable")
	module := media.New(media.Deps{
		Attachments: attachments{err: sentinel},
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

func TestMediaDoesNotOwnMessagePorts(t *testing.T) {
	t.Parallel()

	// Сообщения и подтверждения принадлежат Chat по таблице 11 HLD: модуль
	// Media не должен требовать их в своих зависимостях.
	module := media.New(media.Deps{})
	if module == nil {
		t.Fatal("module was not constructed without message ports")
	}
}
