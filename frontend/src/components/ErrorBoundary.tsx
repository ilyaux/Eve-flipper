import { Component, type ReactNode } from "react";
import { useI18n, type TranslationKey } from "@/lib/i18n";

interface Props {
  children: ReactNode;
  fallback?: ReactNode;
}

interface State {
  hasError: boolean;
  error: Error | null;
}

type TFn = (key: TranslationKey, params?: Record<string, string | number>) => string;

// Inner class component that receives translations via props
class ErrorBoundaryInner extends Component<Props & { t: TFn }, State> {
  constructor(props: Props & { t: TFn }) {
    super(props);
    this.state = { hasError: false, error: null };
  }

  static getDerivedStateFromError(error: Error): State {
    return { hasError: true, error };
  }

  componentDidCatch(error: Error, errorInfo: React.ErrorInfo) {
    console.error("ErrorBoundary caught an error:", error, errorInfo);
  }

  handleRetry = () => {
    this.setState({ hasError: false, error: null });
  };

  render() {
    if (this.state.hasError) {
      if (this.props.fallback) {
        return this.props.fallback;
      }

      const { t } = this.props;

      return (
        <div className="flex flex-col items-center justify-center h-full p-8 bg-eve-dark">
          <div className="max-w-md w-full bg-eve-panel border border-eve-error/50 rounded-sm p-6 text-center">
            <div className="text-4xl mb-4">⚠️</div>
            <h2 className="text-lg font-semibold text-eve-error mb-2">
              {t("errorSomethingWentWrong")}
            </h2>
            <p className="text-sm text-eve-dim mb-4">
              {t("errorTryAgainDesc")}
            </p>
            {this.state.error && (
              <details className="mb-4 text-left">
                <summary className="text-xs text-eve-dim cursor-pointer hover:text-eve-text">
                  {t("errorDetails")}
                </summary>
                <pre className="mt-2 p-2 bg-eve-dark rounded text-xs text-eve-error overflow-auto max-h-32">
                  {this.state.error.message}
                  {this.state.error.stack && (
                    <>
                      {"\n\n"}
                      {this.state.error.stack}
                    </>
                  )}
                </pre>
              </details>
            )}
            <div className="flex gap-2 justify-center">
              <button
                onClick={this.handleRetry}
                className="px-4 py-2 text-sm font-medium bg-eve-accent text-eve-dark rounded-sm hover:bg-eve-accent-hover transition-colors"
              >
                {t("errorTryAgain")}
              </button>
              <button
                onClick={() => window.location.reload()}
                className="px-4 py-2 text-sm font-medium border border-eve-border text-eve-dim rounded-sm hover:text-eve-text hover:border-eve-accent/50 transition-colors"
              >
                {t("errorRefreshPage")}
              </button>
            </div>
          </div>
        </div>
      );
    }

    return this.props.children;
  }
}

// Wrapper that provides i18n via hook
export function ErrorBoundary({ children, fallback }: Props) {
  const { t } = useI18n();
  return (
    <ErrorBoundaryInner t={t} fallback={fallback}>
      {children}
    </ErrorBoundaryInner>
  );
}
