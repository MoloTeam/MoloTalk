import { createContext, useContext, useEffect, useSyncExternalStore, type ReactNode } from "react";

import type { AppInstance } from "./createApp";
import type { AppActions } from "../sync/actions";
import type { AppState } from "../state/store";

const AppContext = createContext<AppInstance | null>(null);

export function AppProvider({ app, children }: { app: AppInstance; children: ReactNode }) {
  useEffect(() => () => app.dispose(), [app]);
  return <AppContext.Provider value={app}>{children}</AppContext.Provider>;
}

function useApp(): AppInstance {
  const app = useContext(AppContext);
  if (!app) {
    throw new Error("AppProvider is missing");
  }
  return app;
}

export function useAppState(): AppState {
  const app = useApp();
  return useSyncExternalStore(app.store.subscribe, app.store.getState, app.store.getState);
}

export function useAppActions(): AppActions {
  return useApp().actions;
}
