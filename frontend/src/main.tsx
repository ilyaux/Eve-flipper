import React from "react";
import ReactDOM from "react-dom/client";
import { I18nProvider } from "./lib/i18n";
import { ThemeProvider } from "./lib/useTheme";
import { ErrorBoundary } from "./components/ErrorBoundary";
import { ToastProvider } from "./components/Toast";
import { installDomSafetyGuards } from "./lib/domSafety";
import App from "./App";
import { CorpDashboardApp } from "./components/CorpDashboardApp";

const isCorpRoute = window.location.pathname.startsWith("/corp");

installDomSafetyGuards();

ReactDOM.createRoot(document.getElementById("root")!).render(
  <React.StrictMode>
    <ThemeProvider>
      <I18nProvider>
        <ErrorBoundary>
          <ToastProvider>
            {isCorpRoute ? <CorpDashboardApp /> : <App />}
          </ToastProvider>
        </ErrorBoundary>
      </I18nProvider>
    </ThemeProvider>
  </React.StrictMode>
);
