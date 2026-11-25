import { useEffect, type ReactNode } from "react";
import { NavLink, Route, Routes, useParams } from "react-router-dom";

import { LoginScreen } from "../features/access/LoginScreen";
import { NotFoundScreen } from "../features/access/NotFoundScreen";
import { ProfileScreen } from "../features/access/ProfileScreen";
import { Composer } from "../features/composer/Composer";
import { DialogsScreen } from "../features/dialogs/DialogsScreen";
import { Timeline } from "../features/timeline/Timeline";
import { useAppActions } from "./AppProvider";

function AppLayout({ children }: { children: ReactNode }) {
  return (
    <div className="app-shell">
      <header className="app-header">
        <strong>MoloTalk</strong>
        <nav>
          <NavLink to="/">Диалоги</NavLink>
          <NavLink to="/login">Вход</NavLink>
          <NavLink to="/profile">Профиль</NavLink>
        </nav>
      </header>
      <main className="app-main">{children}</main>
    </div>
  );
}

function ChatScreen() {
  const { chatId } = useParams<{ chatId: string }>();
  const actions = useAppActions();

  // Подписка живёт ровно столько, сколько открыт диалог.
  useEffect(() => {
    actions.startRealtime();
    return () => actions.stopRealtime();
  }, [actions]);

  return (
    <div className="chat-screen">
      <DialogsScreen />
      <section className="chat-content">
        <Timeline chatId={chatId ?? null} />
        <Composer chatId={chatId ?? null} />
      </section>
    </div>
  );
}

export function AppRoutes() {
  return (
    <Routes>
      <Route
        path="/"
        element={
          <AppLayout>
            <DialogsScreen />
          </AppLayout>
        }
      />
      <Route
        path="/login"
        element={
          <AppLayout>
            <LoginScreen />
          </AppLayout>
        }
      />
      <Route
        path="/profile"
        element={
          <AppLayout>
            <ProfileScreen />
          </AppLayout>
        }
      />
      <Route
        path="/chats"
        element={
          <AppLayout>
            <DialogsScreen />
          </AppLayout>
        }
      />
      <Route
        path="/chats/:chatId"
        element={
          <AppLayout>
            <ChatScreen />
          </AppLayout>
        }
      />
      <Route
        path="*"
        element={
          <AppLayout>
            <NotFoundScreen />
          </AppLayout>
        }
      />
    </Routes>
  );
}
