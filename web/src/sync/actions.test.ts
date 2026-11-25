import { create } from "@bufbuild/protobuf";
import { MessageDeletedSchema, ServerEventSchema, type ServerEvent } from "@api/molotalk/v1/gateway_pb";
import { describe, expect, it } from "vitest";

import { createApp } from "../app/createApp";
import type { ApiPort, SocketHandlers, SocketPort } from "./ports";

class FakeApi implements ApiPort {
  calls = 0;

  constructor(private readonly result: { id: string; displayName: string } | Error) {}

  async getMe() {
    this.calls += 1;
    if (this.result instanceof Error) {
      throw this.result;
    }
    return this.result;
  }
}

class FakeSocket implements SocketPort {
  handlers: SocketHandlers | undefined;
  unsubscribed = 0;

  connect(handlers: SocketHandlers) {
    this.handlers = handlers;
    return () => {
      this.unsubscribed += 1;
      this.handlers = undefined;
    };
  }

  emit(event: ServerEvent) {
    this.handlers?.onEvent(event);
  }
}

function deletedEvent(): ServerEvent {
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

function presenceEvent(): ServerEvent {
  return create(ServerEventSchema, {
    eventId: "01ARZ3NDEKTSV4RRFFQ69G5FA3",
    occurredAt: { seconds: 1795000001n, nanos: 0 },
    event: {
      case: "presenceChanged",
      value: { userId: "01ARZ3NDEKTSV4RRFFQ69G5FA4", status: 1 },
    },
  });
}

describe("actions", () => {
  it("reads through the API port and stores the profile", async () => {
    const api = new FakeApi({ id: "01ARZ3NDEKTSV4RRFFQ69G5FA1", displayName: "Alice" });
    const app = createApp({ api, socket: new FakeSocket() });

    await app.actions.loadProfile();

    expect(api.calls).toBe(1);
    expect(app.store.getState().profile).toEqual({ id: "01ARZ3NDEKTSV4RRFFQ69G5FA1", displayName: "Alice" });
  });

  it("stores a safe category when the API fails", async () => {
    const api = new FakeApi(new Error("secret from dependency"));
    const app = createApp({ api, socket: new FakeSocket() });

    await app.actions.loadProfile();

    expect(app.store.getState().error).toBe("profile_request_failed");
    expect(JSON.stringify(app.store.getState())).not.toContain("secret from dependency");
  });

  it("applies a socket event and drops the subscription on dispose", () => {
    const socket = new FakeSocket();
    const app = createApp({ api: new FakeApi(new Error("unused")), socket });

    app.actions.startRealtime();
    socket.emit(deletedEvent());
    expect(app.store.getState().lastEventType).toBe("messageDeleted");

    app.dispose();
    expect(socket.unsubscribed).toBe(1);

    socket.emit(presenceEvent());
    expect(app.store.getState().lastEventType).toBe("messageDeleted");
  });

  it("selects a chat through the action layer", () => {
    const app = createApp({ api: new FakeApi(new Error("unused")), socket: new FakeSocket() });

    app.actions.selectChat("01ARZ3NDEKTSV4RRFFQ69G5FA5");

    expect(app.store.getState().selectedChatId).toBe("01ARZ3NDEKTSV4RRFFQ69G5FA5");
  });
});
