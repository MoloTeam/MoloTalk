import type { ServerEvent } from "@api/molotalk/v1/gateway_pb";

import type { Profile } from "../state/store";

// Порты — единственная точка, через которую слой обмена говорит с транспортом.
// UI об этих интерфейсах не знает: он вызывает действия приложения.

export interface ApiPort {
  getMe(): Promise<Profile>;
}

export interface SocketHandlers {
  onEvent(event: ServerEvent): void;
  onError?(error: Error): void;
}

export interface SocketPort {
  connect(handlers: SocketHandlers): () => void;
  send?(event: ServerEvent): void;
}
