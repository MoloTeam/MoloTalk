package presence_test

import (
	"context"
	"errors"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/MoloTeam/MoloTalk/backend/internal/platform/clock"
	"github.com/MoloTeam/MoloTalk/backend/internal/platform/idgen"
	"github.com/MoloTeam/MoloTalk/backend/internal/presence"
)

// connectionStore повторяет семантику хранилища: TTL и состояние ведутся по
// подключению, а не по пользователю.
type connectionStore struct {
	mu          sync.Mutex
	connections map[string]presence.Presence
}

func newConnectionStore() *connectionStore {
	return &connectionStore{connections: make(map[string]presence.Presence)}
}

func (s *connectionStore) Renew(_ context.Context, state presence.Presence) error {
	if state.ConnectionID == "" {
		return errors.New("connection id is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.connections[state.ConnectionID] = state
	return nil
}

func (s *connectionStore) Remove(_ context.Context, connectionID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.connections, connectionID)
	return nil
}

func (s *connectionStore) ListLive(_ context.Context, chatID string) ([]presence.Presence, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var live []presence.Presence
	for _, state := range s.connections {
		if state.ChatID == chatID {
			live = append(live, state)
		}
	}
	sort.Slice(live, func(i, j int) bool { return live[i].ConnectionID < live[j].ConnectionID })
	return live, nil
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
	_ presence.PresenceStateStore = (*connectionStore)(nil)
	_ presence.AuthClient         = authClient{}
	_ presence.ChatAccessClient   = chatClient{}
)

func TestPresencePortsWorkWithoutNetwork(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("presence storage unavailable")
	module := presence.New(presence.Deps{
		States: newConnectionStore(),
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

func TestRemovingOneConnectionKeepsTheOther(t *testing.T) {
	t.Parallel()

	store := newConnectionStore()
	ctx := context.Background()

	first := presence.Presence{ConnectionID: "connection-1", AccountID: "alice", ChatID: "chat-1", Until: time.Unix(1000, 0)}
	second := presence.Presence{ConnectionID: "connection-2", AccountID: "alice", ChatID: "chat-1", Until: time.Unix(1000, 0)}

	if err := store.Renew(ctx, first); err != nil {
		t.Fatalf("renew first: %v", err)
	}
	if err := store.Renew(ctx, second); err != nil {
		t.Fatalf("renew second: %v", err)
	}

	if err := store.Remove(ctx, first.ConnectionID); err != nil {
		t.Fatalf("remove first: %v", err)
	}

	live, err := store.ListLive(ctx, "chat-1")
	if err != nil {
		t.Fatalf("list live: %v", err)
	}
	if len(live) != 1 {
		t.Fatalf("live connections = %d, want 1", len(live))
	}
	if live[0].ConnectionID != second.ConnectionID {
		t.Fatalf("live connection = %q, want %q", live[0].ConnectionID, second.ConnectionID)
	}
	if live[0].AccountID != "alice" {
		t.Fatalf("live account = %q, want alice", live[0].AccountID)
	}
}
