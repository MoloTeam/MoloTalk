import type { ApiPort, SocketPort } from "./ports";
import type { Store } from "../state/store";

export interface AppActions {
  selectChat(chatId: string): void;
  loadProfile(): Promise<void>;
  startRealtime(): void;
  stopRealtime(): void;
}

export interface ActionDeps {
  store: Store;
  api: ApiPort;
  socket: SocketPort;
}

export function createActions({ store, api, socket }: ActionDeps): AppActions {
  let unsubscribe: (() => void) | null = null;

  return {
    selectChat(chatId) {
      store.update({ selectedChatId: chatId });
    },

    async loadProfile() {
      try {
        const profile = await api.getMe();
        store.update({ profile, error: null });
      } catch {
        // Текст ошибки транспорта не попадает в состояние: он может содержать
        // служебные данные. Хранится безопасная категория.
        store.update({ error: "profile_request_failed" });
      }
    },

    startRealtime() {
      if (unsubscribe) {
        return;
      }
      store.update({ connection: "connecting" });
      unsubscribe = socket.connect({
        onOpen() {
          store.update({ connection: "open" });
        },
        onFrame(frame) {
          if (frame.case === "ready") {
            store.update({ connection: "ready" });
            return;
          }
          if (frame.case === "serverEvent") {
            store.update({ lastEventType: frame.value.event.case ?? null });
          }
          // HeartbeatAck пока не наблюдаем в состоянии: таймеры вне карточки.
        },
        onClose() {
          store.update({ connection: "closed" });
        },
        onError() {
          store.update({ error: "socket_error" });
        },
      });
    },

    stopRealtime() {
      unsubscribe?.();
      unsubscribe = null;
      store.update({ connection: "closed" });
    },
  };
}
