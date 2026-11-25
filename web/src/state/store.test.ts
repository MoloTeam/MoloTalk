import { describe, expect, it, vi } from "vitest";

import { createStore } from "./store";

describe("store", () => {
  it("keeps instances independent", () => {
    const first = createStore();
    const second = createStore();

    first.update({ selectedChatId: "chat-1" });

    expect(first.getState().selectedChatId).toBe("chat-1");
    expect(second.getState().selectedChatId).toBeNull();
    expect(second.getState().chats).not.toBe(first.getState().chats);
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
