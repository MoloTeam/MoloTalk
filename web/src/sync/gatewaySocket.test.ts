import fs from "node:fs";
import path from "node:path";

import { create, fromBinary, toBinary } from "@bufbuild/protobuf";
import {
  GatewayEnvelopeSchema,
  HeartbeatAckSchema,
  HeartbeatSchema,
  IdentifySchema,
  MessageDeletedSchema,
  ReadySchema,
  ServerEventSchema,
  type GatewayEnvelope,
  type ServerEvent,
} from "@api/molotalk/v1/gateway_pb";
import { describe, expect, it, vi } from "vitest";

import { createGatewaySocket, gatewayUrl, type SocketEventType, type SocketLike } from "./gatewaySocket";
import type { SocketHandlers } from "./ports";

const readyFixture = fs
  .readFileSync(path.resolve(import.meta.dirname, "../../../api/testdata/gateway-server-ready.b64"), "utf8")
  .trim();

class FakeSocket implements SocketLike {
  private currentType = "blob";
  private readonly listeners = new Map<SocketEventType, Set<(event: Event) => void>>();
  sent: Uint8Array[] = [];
  closed = 0;

  constructor(private readonly ignoreBinaryType = false) {}

  get binaryType() {
    return this.currentType;
  }

  set binaryType(value: string) {
    if (!this.ignoreBinaryType) {
      this.currentType = value;
    }
  }

  addEventListener(type: SocketEventType, listener: (event: Event) => void) {
    if (!this.listeners.has(type)) {
      this.listeners.set(type, new Set());
    }
    this.listeners.get(type)?.add(listener);
  }

  removeEventListener(type: SocketEventType, listener: (event: Event) => void) {
    this.listeners.get(type)?.delete(listener);
  }

  send(data: ArrayBufferView) {
    this.sent.push(new Uint8Array(data.buffer, data.byteOffset, data.byteLength));
  }

  close() {
    this.closed += 1;
  }

  // Браузер отдаёт Blob, если binaryType не выставлен в arraybuffer.
  deliverFrame(bytes: Uint8Array) {
    const buffer = new ArrayBuffer(bytes.byteLength);
    new Uint8Array(buffer).set(bytes);
    this.deliver(this.currentType === "arraybuffer" ? buffer : new Blob([buffer]));
  }

  deliver(data: ArrayBuffer | Blob) {
    this.emit("message", { data } as MessageEvent);
  }

  open() {
    this.emit("open", new Event("open"));
  }

  closeFromServer(code: number, reason: string) {
    this.emit("close", { code, reason } as CloseEvent);
  }

  fail() {
    this.emit("error", new Event("error"));
  }

  private emit(type: SocketEventType, event: Event) {
    for (const listener of this.listeners.get(type) ?? []) {
      listener(event);
    }
  }
}

function bytes(payload: GatewayEnvelope["payload"]): Uint8Array {
  return toBinary(GatewayEnvelopeSchema, create(GatewayEnvelopeSchema, { payload }));
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

function setup(ignoreBinaryType = false) {
  const socket = new FakeSocket(ignoreBinaryType);
  const handlers: SocketHandlers = { onFrame: vi.fn(), onError: vi.fn() };
  const port = createGatewaySocket({
    origin: "https://example.test",
    createSocket: () => socket,
  });
  const release = port.connect(handlers);
  return { socket, handlers, port, release };
}

describe("gatewaySocket", () => {
  it("builds ws/wss from the application origin", () => {
    expect(gatewayUrl("https://example.test")).toBe("wss://example.test/ws");
    expect(gatewayUrl("http://localhost:5173")).toBe("ws://localhost:5173/ws");
  });

  it("sets binaryType before any frame arrives", () => {
    const { socket, handlers } = setup();

    expect(socket.binaryType).toBe("arraybuffer");

    socket.deliverFrame(bytes({ case: "ready", value: create(ReadySchema, { userId: "01ARZ3NDEKTSV4RRFFQ69G5FA1" }) }));
    expect(handlers.onFrame).toHaveBeenCalledTimes(1);
  });

  it("reports a Blob frame when binaryType was not applied", () => {
    const { socket, handlers } = setup(true);

    socket.deliverFrame(bytes({ case: "heartbeatAck", value: create(HeartbeatAckSchema, {}) }));

    expect(handlers.onFrame).not.toHaveBeenCalled();
    expect(handlers.onError).toHaveBeenCalledWith(expect.objectContaining({ message: "gateway frame must be binary" }));
  });

  it("delivers Ready and HeartbeatAck frames", () => {
    const { socket, handlers } = setup();

    socket.deliverFrame(bytes({ case: "ready", value: create(ReadySchema, { userId: "01ARZ3NDEKTSV4RRFFQ69G5FA1" }) }));
    socket.deliverFrame(bytes({ case: "heartbeatAck", value: create(HeartbeatAckSchema, {}) }));

    const calls = (handlers.onFrame as ReturnType<typeof vi.fn>).mock.calls;
    expect(calls[0][0].case).toBe("ready");
    expect(calls[0][0].value.userId).toBe("01ARZ3NDEKTSV4RRFFQ69G5FA1");
    expect(calls[1][0].case).toBe("heartbeatAck");
  });

  it("delivers ServerEvent frames", () => {
    const { socket, handlers } = setup();

    socket.deliverFrame(bytes({ case: "serverEvent", value: event() }));

    const frame = (handlers.onFrame as ReturnType<typeof vi.fn>).mock.calls[0][0];
    expect(frame.case).toBe("serverEvent");
    expect(frame.value.event.case).toBe("messageDeleted");
  });

  it("treats client-only frames from the server as protocol violations", () => {
    const { socket, handlers } = setup();

    socket.deliverFrame(bytes({ case: "identify", value: create(IdentifySchema, { token: "token" }) }));
    socket.deliverFrame(bytes({ case: "heartbeat", value: create(HeartbeatSchema, {}) }));

    expect(handlers.onFrame).not.toHaveBeenCalled();
    expect(handlers.onError).toHaveBeenCalledWith(expect.objectContaining({ message: "unexpected identify frame from server" }));
    expect(handlers.onError).toHaveBeenCalledWith(expect.objectContaining({ message: "unexpected heartbeat frame from server" }));
  });

  it("rejects an envelope without a payload", () => {
    const { socket, handlers } = setup();

    socket.deliverFrame(toBinary(GatewayEnvelopeSchema, create(GatewayEnvelopeSchema, {})));

    expect(handlers.onFrame).not.toHaveBeenCalled();
    expect(handlers.onError).toHaveBeenCalledWith(expect.objectContaining({ message: "gateway frame has no payload" }));
  });

  it("does not deliver a corrupted frame", () => {
    const { socket, handlers } = setup();

    socket.deliverFrame(new Uint8Array([0xff, 0x01, 0x02]));

    expect(handlers.onFrame).not.toHaveBeenCalled();
    expect(handlers.onError).toHaveBeenCalledTimes(1);
  });

  it("forwards socket open, close code and error", () => {
    const socket = new FakeSocket();
    const handlers: SocketHandlers = { onFrame: vi.fn(), onOpen: vi.fn(), onClose: vi.fn(), onError: vi.fn() };
    const port = createGatewaySocket({ origin: "https://example.test", createSocket: () => socket });
    port.connect(handlers);

    socket.open();
    socket.closeFromServer(4008, "idle timeout");
    socket.fail();

    expect(handlers.onOpen).toHaveBeenCalledTimes(1);
    expect(handlers.onClose).toHaveBeenCalledWith(4008, "idle timeout");
    expect(handlers.onError).toHaveBeenCalledWith(expect.objectContaining({ message: "gateway socket error" }));
  });

  it("encodes a client Identify frame and closes on release", () => {
    const { socket, handlers, port, release } = setup();

    port.send?.({ case: "identify", value: create(IdentifySchema, { token: "token" }) });

    expect(socket.sent).toHaveLength(1);
    const envelope = fromBinary(GatewayEnvelopeSchema, socket.sent[0]);
    expect(envelope.payload.case).toBe("identify");
    if (envelope.payload.case === "identify") {
      expect(envelope.payload.value.token).toBe("token");
    }

    release();
    expect(socket.closed).toBe(1);

    socket.deliverFrame(bytes({ case: "serverEvent", value: event() }));
    expect(handlers.onFrame).not.toHaveBeenCalled();
  });

  it("encodes a client Heartbeat frame", () => {
    const { socket, port } = setup();

    port.send?.({ case: "heartbeat", value: create(HeartbeatSchema, {}) });

    const envelope = fromBinary(GatewayEnvelopeSchema, socket.sent[0]);
    expect(envelope.payload.case).toBe("heartbeat");
  });

  it("decodes the Ready fixture produced by Go", () => {
    const decoded = fromBinary(GatewayEnvelopeSchema, Uint8Array.from(Buffer.from(readyFixture, "base64")));

    expect(decoded.payload.case).toBe("ready");
    if (decoded.payload.case !== "ready") {
      return;
    }
    expect(decoded.payload.value.userId).toBe("01ARZ3NDEKTSV4RRFFQ69G5FA1");
    expect(decoded.payload.value.chatIds).toHaveLength(2);
    expect(decoded.payload.value.readStates[0].unreadCount).toBe(3);
  });

  it("delivers the Ready fixture through the adapter", () => {
    const { socket, handlers } = setup();

    socket.deliverFrame(Uint8Array.from(Buffer.from(readyFixture, "base64")));

    const frame = (handlers.onFrame as ReturnType<typeof vi.fn>).mock.calls[0][0];
    expect(frame.case).toBe("ready");
    expect(frame.value.chatIds).toContain("01ARZ3NDEKTSV4RRFFQ69G5FA5");
  });
});
