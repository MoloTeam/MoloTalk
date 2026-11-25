import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, describe, expect, it, vi } from "vitest";

import { AppProvider } from "../../app/AppProvider";
import { createApp } from "../../app/createApp";
import type { ApiPort, SocketPort } from "../../sync/ports";
import { DialogsScreen } from "./DialogsScreen";

const api: ApiPort = {
  getMe: async () => ({ id: "01ARZ3NDEKTSV4RRFFQ69G5FA1", displayName: "Alice" }),
};
const socket: SocketPort = { connect: () => () => {} };

afterEach(cleanup);

describe("DialogsScreen", () => {
  it("passes the selected chat id to the action", () => {
    const app = createApp({ api, socket });
    const selectChat = vi.spyOn(app.actions, "selectChat");

    render(
      <AppProvider app={app}>
        <MemoryRouter>
          <DialogsScreen />
        </MemoryRouter>
      </AppProvider>,
    );

    fireEvent.click(screen.getByRole("button", { name: /Alice/ }));

    expect(selectChat).toHaveBeenCalledWith("01ARZ3NDEKTSV4RRFFQ69G5FA4");
    expect(app.store.getState().selectedChatId).toBe("01ARZ3NDEKTSV4RRFFQ69G5FA4");
  });

  it("marks the selected chat", () => {
    const app = createApp({ api, socket, initial: { selectedChatId: "01ARZ3NDEKTSV4RRFFQ69G5FA5" } });

    render(
      <AppProvider app={app}>
        <MemoryRouter>
          <DialogsScreen />
        </MemoryRouter>
      </AppProvider>,
    );

    const items = screen.getAllByRole("listitem");
    expect(items[1].className).toBe("selected");
  });
});
