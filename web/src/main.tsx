import React from "react";
import { createRoot } from "react-dom/client";
import { BrowserRouter } from "react-router-dom";

import { AppProvider } from "./app/AppProvider";
import { AppRoutes } from "./app/App";
import { createApp } from "./app/createApp";
import "./styles.css";

const app = createApp();
const container = document.getElementById("root");

if (!container) {
  throw new Error("root element is missing");
}

createRoot(container).render(
  <React.StrictMode>
    <AppProvider app={app}>
      <BrowserRouter>
        <AppRoutes />
      </BrowserRouter>
    </AppProvider>
  </React.StrictMode>,
);
