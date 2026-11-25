import { describe, expect, it, vi } from "vitest";

import { createStore, type AppState } from "./store";

describe("store", () => {
  it("keeps instances independent", () => {
    const first = createStore();
    const second = createStore();

    first.update({ selectedChatId: "chat-1" });

    expect(first.getState().selectedChatId).toBe("chat-1");
    expect(second.getState().selectedChatId).toBeNull();
    expect(second.getState().connection).toBe("closed");
    expect(second.getState().chats).not.toBe(first.getState().chats);
  });

  it("clones provided initial collections for every instance", () => {
    const initial: Pick<AppState, "chats" | "messages"> = {
      chats: [{ id: "chat-1", title: "One", kind: "direct" }],
      messages: { "chat-1": ["hello"] },
    };

    const first = createStore(initial);
    const second = createStore(initial);

    expect(first.getState().chats).not.toBe(second.getState().chats);
    expect(first.getState().chats).not.toBe(initial.chats);
    expect(first.getState().messages["chat-1"]).not.toBe(initial.messages["chat-1"]);

    initial.chats.push({ id: "chat-2", title: "Two", kind: "group" });
    initial.messages["chat-1"].push("later");

    expect(first.getState().chats).toHaveLength(1);
    expect(second.getState().chats).toHaveLength(1);
    expect(first.getState().messages["chat-1"]).toEqual(["hello"]);
  });

  it("notifies subscribers and stops after unsubscribe", () => {
    const store = createStore();
    const listener = vi.fn();
    const unsubscribe = store.subscribe(listener);

    store.update({ error: "boom" });
    expect(listener).toHaveBeenCalledTimes(1);

    unsubscribe();
    store.update({ error: null });
    expect(listener).toHaveBeenCalledTimes(1);
  });

  it("applies functional updates to the current state", () => {
    const store = createStore({ selectedChatId: "a" });

    store.update((state) => ({ selectedChatId: `${state.selectedChatId}-next` }));

    expect(store.getState().selectedChatId).toBe("a-next");
  });
});
