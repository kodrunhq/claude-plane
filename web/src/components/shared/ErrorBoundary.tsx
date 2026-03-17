import { Component, type ErrorInfo, type ReactNode } from 'react';
import { AlertTriangle } from 'lucide-react';

interface ErrorBoundaryProps {
  children: ReactNode;
}

interface ErrorBoundaryState {
  readonly hasError: boolean;
  readonly error: Error | null;
}

export class ErrorBoundary extends Component<ErrorBoundaryProps, ErrorBoundaryState> {
  constructor(props: ErrorBoundaryProps) {
    super(props);
    this.state = { hasError: false, error: null };
  }

  static getDerivedStateFromError(error: Error): ErrorBoundaryState {
    return { hasError: true, error };
  }

  componentDidCatch(error: Error, errorInfo: ErrorInfo) {
    console.error('[ErrorBoundary] Uncaught error:', error, errorInfo);
  }

  private handleGoHome = () => {
    this.setState({ hasError: false, error: null });
    window.location.href = '/';
  };

  render() {
    if (this.state.hasError) {
      return (
        <div className="flex flex-col items-center justify-center min-h-[60vh] px-4 text-center">
          <div className="text-status-error mb-6 opacity-70">
            <AlertTriangle size={64} />
          </div>
          <h1 className="text-2xl font-bold text-text-primary mb-2">Something went wrong</h1>
          <p className="text-sm text-text-secondary max-w-md mb-4">
            An unexpected error occurred. You can try going back to the home page.
          </p>
          {import.meta.env.DEV && this.state.error?.message && (
            <pre className="text-xs font-mono text-text-secondary bg-bg-tertiary border border-border-primary rounded-lg px-4 py-3 mb-6 max-w-lg overflow-auto">
              {this.state.error.message}
            </pre>
          )}
          <button
            type="button"
            onClick={this.handleGoHome}
            className="px-4 py-2.5 rounded-lg bg-accent-primary text-white text-sm font-medium hover:bg-accent-primary/90 hover:shadow-[0_0_20px_rgba(59,130,246,0.3)] transition-all"
          >
            Go Home
          </button>
        </div>
      );
    }

    return this.props.children;
  }
}
