import type { GatewayEnvelope } from "@api/molotalk/v1/gateway_pb";

import type { Profile } from "../state/store";

// Порты — единственная точка, через которую слой обмена говорит с транспортом.
// UI об этих интерфейсах не знает: он вызывает действия приложения.

export interface ApiPort {
  getMe(): Promise<Profile>;
}

// Серверные кадры Gateway: всё, что браузер получает от сервера, кроме ошибок.
export type ServerFrame = Extract<
  GatewayEnvelope["payload"],
  { case: "ready" | "heartbeatAck" | "serverEvent" }
>;

// Браузер отправляет только Identify и Heartbeat; сервер может присылать
// Ready, HeartbeatAck и ServerEvent.
export interface SocketHandlers {
  onFrame(frame: ServerFrame): void;
  onOpen?(): void;
  onClose?(code: number, reason: string): void;
  onError?(error: Error): void;
}

export type ClientFrame = Extract<
  GatewayEnvelope["payload"],
  { case: "identify" | "heartbeat" }
>;

export interface SocketPort {
  connect(handlers: SocketHandlers): () => void;
  send?(frame: ClientFrame): void;
}
