import { createConnectApi } from "../sync/connectApi";
import { createGatewaySocket } from "../sync/gatewaySocket";
import { createActions, type AppActions } from "../sync/actions";
import type { ApiPort, SocketPort } from "../sync/ports";
import { createStore, type AppState, type Store } from "../state/store";

export interface AppInstance {
  store: Store;
  actions: AppActions;
  dispose(): void;
}

export interface CreateAppOptions {
  api?: ApiPort;
  socket?: SocketPort;
  initial?: Partial<AppState>;
}

// Точка сборки: создаёт независимый экземпляр приложения. В тестах сюда
// передаются подстановки, в браузере — производственные адаптеры.
export function createApp(options: CreateAppOptions = {}): AppInstance {
  const store = createStore(options.initial);
  const api = options.api ?? createConnectApi();
  const socket = options.socket ?? createGatewaySocket();
  const actions = createActions({ store, api, socket });

  return {
    store,
    actions,
    dispose() {
      actions.stopRealtime();
    },
  };
}
