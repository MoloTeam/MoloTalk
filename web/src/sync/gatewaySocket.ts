import { create, fromBinary, toBinary } from "@bufbuild/protobuf";
import { GatewayEnvelopeSchema, type ServerEvent } from "@api/molotalk/v1/gateway_pb";

import type { SocketPort } from "./ports";

export interface SocketLike {
  binaryType: string;
  addEventListener(type: "message", listener: (event: MessageEvent) => void): void;
  removeEventListener(type: "message", listener: (event: MessageEvent) => void): void;
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
      current.binaryType = "arraybuffer";
      socket = current;

      const onMessage = (event: MessageEvent) => {
        if (!(event.data instanceof ArrayBuffer)) {
          handlers.onError?.(new Error("gateway frame must be binary"));
          return;
        }
        try {
          const envelope = fromBinary(GatewayEnvelopeSchema, new Uint8Array(event.data));
          if (envelope.payload.case === "serverEvent") {
            handlers.onEvent(envelope.payload.value);
          }
        } catch (error) {
          // Повреждённый кадр не выдаётся как событие.
          handlers.onError?.(error instanceof Error ? error : new Error("invalid gateway frame"));
        }
      };

      current.addEventListener("message", onMessage);

      return () => {
        current.removeEventListener("message", onMessage);
        current.close();
        if (socket === current) {
          socket = null;
        }
      };
    },

    send(event: ServerEvent) {
      const envelope = create(GatewayEnvelopeSchema, {
        payload: { case: "serverEvent", value: event },
      });
      socket?.send(toBinary(GatewayEnvelopeSchema, envelope));
    },
  };
}
