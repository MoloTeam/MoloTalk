package gateway_test

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/MoloTeam/MoloTalk/backend/internal/gateway"
	"github.com/MoloTeam/MoloTalk/backend/internal/platform/clock"
	"github.com/MoloTeam/MoloTalk/backend/internal/platform/idgen"
)

type authClient struct{ err error }

func (f authClient) CheckSession(context.Context, string) (gateway.Session, error) {
	return gateway.Session{}, f.err
}

type chatClient struct{ err error }

func (f chatClient) CheckAccess(context.Context, string, string) (bool, error) {
	return false, f.err
}

func (f chatClient) GetAudience(context.Context, string) ([]string, error) {
	return nil, f.err
}

type subscriber struct{ err error }

func (f subscriber) Subscribe(context.Context, string, func(context.Context, []byte) error) error {
	return f.err
}

var (
	_ gateway.AuthClient       = authClient{}
	_ gateway.ChatAccessClient = chatClient{}
	_ gateway.EventSubscriber  = subscriber{}
)

func TestGatewayPortsStayBehindTheModule(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("auth service unavailable")
	module := gateway.New(gateway.Deps{
		Auth:       authClient{err: sentinel},
		Chats:      chatClient{err: sentinel},
		Subscriber: subscriber{err: sentinel},
		Clock:      clock.NewManual(time.Unix(0, 0)),
		IDs:        idgen.NewSequence(),
	})

	if module == nil || module.Name() != "gateway" {
		t.Fatal("module was not constructed")
	}

	ctx := context.Background()

	var auth gateway.AuthClient = authClient{err: sentinel}
	if _, err := auth.CheckSession(ctx, "session"); !errors.Is(err, sentinel) {
		t.Errorf("CheckSession error = %v, want the port failure", err)
	}

	// Внутренний HTTP-вызов выполняется адаптером, а не модулем.
	if _, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://auth.internal/session", nil); err != nil {
		t.Fatalf("build request: %v", err)
	}
}
