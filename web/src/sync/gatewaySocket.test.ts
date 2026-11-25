import { create, fromBinary, toBinary } from "@bufbuild/protobuf";
import {
  GatewayEnvelopeSchema,
  HeartbeatSchema,
  IdentifySchema,
  MessageDeletedSchema,
  ServerEventSchema,
  type ServerEvent,
} from "@api/molotalk/v1/gateway_pb";
import { describe, expect, it, vi } from "vitest";

import { createGatewaySocket, gatewayUrl, type SocketLike } from "./gatewaySocket";
import type { SocketHandlers } from "./ports";

class FakeSocket implements SocketLike {
  binaryType = "blob";
  sent: Uint8Array[] = [];
  closed = 0;
  private listener: ((event: MessageEvent) => void) | undefined;

  addEventListener(_type: "message", listener: (event: MessageEvent) => void) {
    this.listener = listener;
  }

  removeEventListener(_type: "message", listener: (event: MessageEvent) => void) {
    if (this.listener === listener) {
      this.listener = undefined;
    }
  }

  send(data: ArrayBufferView) {
    this.sent.push(new Uint8Array(data.buffer, data.byteOffset, data.byteLength));
  }

  close() {
    this.closed += 1;
  }

  deliver(data: ArrayBuffer) {
    this.listener?.({ data } as MessageEvent);
  }
}

function event(): ServerEvent {
  return create(ServerEventSchema, {
    eventId: "01ARZ3NDEKTSV4RRFFQ69G5FA1",
    occurredAt: { seconds: 1795000000n, nanos: 0 },
    event: {
      case: "messageDeleted",
      value: create(MessageDeletedSchema, {
        chatId: "01ARZ3NDEKTSV4RRFFQ69G5FA4",
        messageId: "01ARZ3NDEKTSV4RRFFQ69G5FA2",
        revision: 2,
      }),
    },
  });
}

function frame(payload: ServerEvent): ArrayBuffer {
  const envelope = create(GatewayEnvelopeSchema, {
    payload: { case: "serverEvent", value: payload },
  });
  const bytes = toBinary(GatewayEnvelopeSchema, envelope);
  return bytes.buffer.slice(bytes.byteOffset, bytes.byteOffset + bytes.byteLength) as ArrayBuffer;
}

describe("gatewaySocket", () => {
  it("builds ws/wss from the application origin", () => {
    expect(gatewayUrl("https://example.test")).toBe("wss://example.test/ws");
    expect(gatewayUrl("http://localhost:5173")).toBe("ws://localhost:5173/ws");
  });

  it("decodes a valid binary frame", () => {
    const socket = new FakeSocket();
    const handlers: SocketHandlers = { onEvent: vi.fn(), onError: vi.fn() };
    const port = createGatewaySocket({ origin: "https://example.test", createSocket: () => socket });

    port.connect(handlers);
    socket.deliver(frame(event()));

    expect(handlers.onEvent).toHaveBeenCalledTimes(1);
    const received = (handlers.onEvent as ReturnType<typeof vi.fn>).mock.calls[0][0] as ServerEvent;
    expect(received.event.case).toBe("messageDeleted");
    expect(received.eventId).toBe("01ARZ3NDEKTSV4RRFFQ69G5FA1");
  });

  it("does not emit a corrupted frame as an event", () => {
    const socket = new FakeSocket();
    const handlers: SocketHandlers = { onEvent: vi.fn(), onError: vi.fn() };
    const port = createGatewaySocket({ origin: "https://example.test", createSocket: () => socket });

    port.connect(handlers);
    socket.deliver(new Uint8Array([0xff, 0x01, 0x02]).buffer as ArrayBuffer);

    expect(handlers.onEvent).not.toHaveBeenCalled();
    expect(handlers.onError).toHaveBeenCalledTimes(1);
  });

  it("encodes a client Identify frame and closes on release", () => {
    const socket = new FakeSocket();
    const handlers: SocketHandlers = { onEvent: vi.fn() };
    const port = createGatewaySocket({ origin: "https://example.test", createSocket: () => socket });

    const release = port.connect(handlers);
    port.send?.({ case: "identify", value: create(IdentifySchema, { token: "token" }) });

    expect(socket.sent).toHaveLength(1);
    const envelope = fromBinary(GatewayEnvelopeSchema, socket.sent[0]);
    expect(envelope.payload.case).toBe("identify");
    if (envelope.payload.case === "identify") {
      expect(envelope.payload.value.token).toBe("token");
    }

    release();
    expect(socket.closed).toBe(1);

    socket.deliver(frame(event()));
    expect(handlers.onEvent).not.toHaveBeenCalled();
  });

  it("encodes a client Heartbeat frame", () => {
    const socket = new FakeSocket();
    const port = createGatewaySocket({ origin: "https://example.test", createSocket: () => socket });

    port.connect({ onEvent: vi.fn() });
    port.send?.({ case: "heartbeat", value: create(HeartbeatSchema, {}) });

    const envelope = fromBinary(GatewayEnvelopeSchema, socket.sent[0]);
    expect(envelope.payload.case).toBe("heartbeat");
  });
});
