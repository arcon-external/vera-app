import { Component } from "react";
import type { ErrorInfo, ReactNode } from "react";

/**
 * Contains a render crash instead of letting it take the window with it.
 *
 * Without a boundary, any throw during render unmounts the entire React tree
 * and the user is left with a blank white window and no way forward — which is
 * exactly what a null SubItems array did after Prepare. The app is a desktop
 * tool holding unsaved work, so the priority is that the failure is visible,
 * recoverable, and does not discard the quotation.
 */
interface ErrorBoundaryProps {
  children: ReactNode;
}

interface ErrorBoundaryState {
  error: Error | null;
}

export class ErrorBoundary extends Component<
  ErrorBoundaryProps,
  ErrorBoundaryState
> {
  state: ErrorBoundaryState = { error: null };

  static getDerivedStateFromError(error: Error): ErrorBoundaryState {
    return { error };
  }

  componentDidCatch(error: Error, info: ErrorInfo) {
    // Stays on the console for a bug report; the window shows the short form.
    console.error("Vera render error:", error, info.componentStack);
  }

  render() {
    const { error } = this.state;
    if (!error) return this.props.children;
    return (
      <div className="flex min-h-screen items-center justify-center bg-paper px-6">
        <div className="w-full max-w-lg rounded-xl border border-rule bg-white p-6 shadow-card">
          <h1 className="text-title font-semibold text-ink-950">
            Something went wrong on this screen
          </h1>
          <p className="mt-2 text-small leading-relaxed text-ink-600">
            Vera hit an unexpected error while drawing this page. Your
            quotation is still held in memory — reloading is safe, and anything
            you had saved as a draft is untouched.
          </p>
          <pre className="mt-4 max-h-40 overflow-auto rounded-lg bg-paper-subtle p-3 text-caption text-ink-700">
            {error.message}
          </pre>
          <div className="mt-5 flex flex-wrap gap-2">
            <button
              type="button"
              onClick={() => this.setState({ error: null })}
              className="inline-flex min-h-9 cursor-pointer items-center rounded-lg border border-rule-strong bg-white px-4 py-2.5 text-small text-ink-700 transition-colors duration-150 hover:border-ink-300 hover:text-ink-900"
            >
              Try again
            </button>
            <button
              type="button"
              onClick={() => window.location.reload()}
              className="inline-flex min-h-9 cursor-pointer items-center rounded-lg bg-accent-700 px-4 py-2.5 text-small font-medium text-white transition-colors duration-150 hover:bg-accent-600"
            >
              Reload
            </button>
          </div>
        </div>
      </div>
    );
  }
}
