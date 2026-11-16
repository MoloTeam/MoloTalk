package presence_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/MoloTeam/MoloTalk/backend/internal/platform/clock"
	"github.com/MoloTeam/MoloTalk/backend/internal/platform/idgen"
	"github.com/MoloTeam/MoloTalk/backend/internal/presence"
)

type states struct{ err error }

func (f states) Renew(context.Context, presence.Presence) error { return f.err }

func (f states) Remove(context.Context, string) error { return f.err }

func (f states) ListLive(context.Context, string) ([]presence.Presence, error) {
	return nil, f.err
}

type authClient struct{ err error }

func (f authClient) CheckSession(context.Context, string) (bool, error) { return false, f.err }

type chatClient struct{ err error }

func (f chatClient) CheckAccess(context.Context, string, string) (bool, error) {
	return false, f.err
}

func (f chatClient) GetAudience(context.Context, string) ([]string, error) {
	return nil, f.err
}

var (
	_ presence.PresenceStateStore = states{}
	_ presence.AuthClient         = authClient{}
	_ presence.ChatAccessClient   = chatClient{}
)

func TestPresencePortsWorkWithoutNetwork(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("presence storage unavailable")
	module := presence.New(presence.Deps{
		States: states{err: sentinel},
		Auth:   authClient{err: sentinel},
		Chats:  chatClient{err: sentinel},
		Clock:  clock.NewManual(time.Unix(0, 0)),
		IDs:    idgen.NewSequence(),
	})

	if module == nil || module.Name() != "presence" {
		t.Fatal("module was not constructed")
	}

	ctx := context.Background()

	var auth presence.AuthClient = authClient{err: sentinel}
	if _, err := auth.CheckSession(ctx, "session"); !errors.Is(err, sentinel) {
		t.Errorf("CheckSession error = %v, want the port failure", err)
	}

	var chats presence.ChatAccessClient = chatClient{err: sentinel}
	if _, err := chats.GetAudience(ctx, "chat"); !errors.Is(err, sentinel) {
		t.Errorf("GetAudience error = %v, want the port failure", err)
	}
}
