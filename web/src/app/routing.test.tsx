import { cleanup, render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, describe, expect, it } from "vitest";

import { AppRoutes } from "./App";
import { AppProvider } from "./AppProvider";
import { createApp } from "./createApp";
import type { ApiPort, SocketPort } from "../sync/ports";

const api: ApiPort = { getMe: async () => ({ id: "u", displayName: "U" }) };
const socket: SocketPort = { connect: () => () => {} };

const chatId = "01ARZ3NDEKTSV4RRFFQ69G5FA4";

function renderAt(path: string) {
  const app = createApp({ api, socket });
  return render(
    <AppProvider app={app}>
      <MemoryRouter initialEntries={[path]}>
        <AppRoutes />
      </MemoryRouter>
    </AppProvider>,
  );
}

afterEach(cleanup);

describe("routes", () => {
  it("opens a direct chat route and gives the same chat id to history and composer", () => {
    renderAt(`/chats/${chatId}`);

    expect(screen.getByRole("region", { name: "История и статусы" }).textContent).toContain(`Диалог: ${chatId}`);
    expect(screen.getByRole("region", { name: "Редактор" }).textContent).toContain(`Диалог: ${chatId}`);
  });

  it("renders the login screen", () => {
    renderAt("/login");

    expect(screen.getByRole("heading", { name: "Вход" })).toBeDefined();
  });

  it("shows the not-found screen for an unknown path", () => {
    renderAt("/unknown/path");

    expect(screen.getByRole("heading", { name: "Страница не найдена" })).toBeDefined();
  });
});
