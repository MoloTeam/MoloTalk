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
      unsubscribe = socket.connect({
        onEvent(event) {
          store.update({ lastEventType: event.event.case ?? null });
        },
        onError() {
          store.update({ error: "socket_frame_rejected" });
        },
      });
    },

    stopRealtime() {
      unsubscribe?.();
      unsubscribe = null;
    },
  };
}
