export interface ChatSummary {
  id: string;
  title: string;
  kind: "direct" | "group";
}

export interface Profile {
  id: string;
  displayName: string;
}

export interface AppState {
  profile: Profile | null;
  chats: ChatSummary[];
  selectedChatId: string | null;
  messages: Record<string, string[]>;
  lastEventType: string | null;
  error: string | null;
}

export type Listener = () => void;

export interface Store {
  getState(): AppState;
  update(partial: Partial<AppState> | ((state: AppState) => Partial<AppState>)): void;
  subscribe(listener: Listener): () => void;
}

// Начальное состояние создаётся заново для каждого экземпляра: общих
// Изменяемые вложенные сущности копируются: переданный набор данных не должен
// становиться общим для двух экземпляров приложения.
function cloneState(state: AppState): AppState {
  return {
    ...state,
    profile: state.profile ? { ...state.profile } : null,
    chats: state.chats.map((chat) => ({ ...chat })),
    messages: Object.fromEntries(
      Object.entries(state.messages).map(([chatId, ids]) => [chatId, [...ids]]),
    ),
  };
}

// изменяемых коллекций между вкладками и тестами быть не должно.
export function createInitialState(overrides: Partial<AppState> = {}): AppState {
  return cloneState({
    profile: null,
    chats: [
      { id: "01ARZ3NDEKTSV4RRFFQ69G5FA4", title: "Alice", kind: "direct" },
      { id: "01ARZ3NDEKTSV4RRFFQ69G5FA5", title: "Team", kind: "group" },
    ],
    selectedChatId: null,
    messages: {},
    lastEventType: null,
    error: null,
    ...overrides,
  });
}

export function createStore(initial: Partial<AppState> = {}): Store {
  let state = createInitialState(initial);
  const listeners = new Set<Listener>();

  return {
    getState: () => state,
    update(partial) {
      const next = typeof partial === "function" ? partial(state) : partial;
      state = { ...state, ...next };
      for (const listener of [...listeners]) {
        listener();
      }
    },
    subscribe(listener) {
      listeners.add(listener);
      return () => {
        listeners.delete(listener);
      };
    },
  };
}
