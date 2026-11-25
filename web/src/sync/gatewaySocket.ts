import { create, fromBinary, toBinary } from "@bufbuild/protobuf";
import { GatewayEnvelopeSchema } from "@api/molotalk/v1/gateway_pb";

import type { ClientFrame, SocketPort } from "./ports";

export type SocketEventType = "message" | "open" | "close" | "error";

export interface SocketLike {
  binaryType: string;
  addEventListener(type: SocketEventType, listener: (event: Event) => void): void;
  removeEventListener(type: SocketEventType, listener: (event: Event) => void): void;
  send(data: ArrayBufferView): void;
  close(): void;
}

export type SocketFactory = (url: string) => SocketLike;

export interface GatewaySocketOptions {
  origin?: string;
  createSocket?: SocketFactory;
}

// Адрес строится из origin приложения: https → wss, http → ws.
export function gatewayUrl(origin: string): string {
  const url = new URL("/ws", origin);
  url.protocol = url.protocol === "https:" ? "wss:" : "ws:";
  return url.toString();
}

export function createGatewaySocket(options: GatewaySocketOptions = {}): SocketPort {
  const factory: SocketFactory = options.createSocket ?? ((url) => new WebSocket(url));
  const url = gatewayUrl(options.origin ?? window.location.origin);
  let socket: SocketLike | null = null;

  return {
    connect(handlers) {
      const current = factory(url);
      // Выставляется до подписки: иначе браузер отдаёт кадры как Blob.
      current.binaryType = "arraybuffer";
      socket = current;

      const onOpen = () => {
        handlers.onOpen?.();
      };

      const onMessage = (event: Event) => {
        const data = (event as MessageEvent).data;
        if (!(data instanceof ArrayBuffer)) {
          handlers.onError?.(new Error("gateway frame must be binary"));
          return;
        }

        let payload;
        try {
          payload = fromBinary(GatewayEnvelopeSchema, new Uint8Array(data)).payload;
        } catch (error) {
          // Повреждённый кадр не доставляется как кадр.
          handlers.onError?.(error instanceof Error ? error : new Error("invalid gateway frame"));
          return;
        }

        switch (payload.case) {
          case "ready":
          case "heartbeatAck":
          case "serverEvent":
            handlers.onFrame(payload);
            return;
          case "identify":
          case "heartbeat":
            // Эти кадры отправляет только браузер.
            handlers.onError?.(new Error(`unexpected ${payload.case} frame from server`));
            return;
          default:
            handlers.onError?.(new Error("gateway frame has no payload"));
        }
      };

      const onClose = (event: Event) => {
        // Код и причина передаются без интерпретации: политика переподключения
        // вне этой карточки.
        const closeEvent = event as CloseEvent;
        handlers.onClose?.(closeEvent.code, closeEvent.reason);
        if (socket === current) {
          socket = null;
        }
      };

      const onError = () => {
        handlers.onError?.(new Error("gateway socket error"));
      };

      current.addEventListener("open", onOpen);
      current.addEventListener("message", onMessage);
      current.addEventListener("close", onClose);
      current.addEventListener("error", onError);

      return () => {
        current.removeEventListener("open", onOpen);
        current.removeEventListener("message", onMessage);
        current.removeEventListener("close", onClose);
        current.removeEventListener("error", onError);
        current.close();
        if (socket === current) {
          socket = null;
        }
      };
    },

    send(frame: ClientFrame) {
      const envelope = create(GatewayEnvelopeSchema, { payload: frame });
      socket?.send(toBinary(GatewayEnvelopeSchema, envelope));
    },
  };
}
