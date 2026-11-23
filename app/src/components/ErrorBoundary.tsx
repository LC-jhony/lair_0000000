import { Component, type ErrorInfo, type ReactNode } from 'react';
import { RefreshCw } from 'lucide-react';
import { useT } from '../i18n';
import { Button } from './ui/ActionButton';

interface ErrorBoundaryProps {
  children: ReactNode;
  title: string;
  message: string;
  retryLabel: string;
  onRetry?: () => void;
}

/** Keeps a crash in one pane from taking down the whole desktop window. */
export class ErrorBoundary extends Component<ErrorBoundaryProps, { error: Error | null }> {
  state: { error: Error | null } = { error: null };

  static getDerivedStateFromError(error: Error) {
    return { error };
  }

  componentDidCatch(error: Error, info: ErrorInfo) {
    console.error('Lair pane crashed', error, info);
  }

  private retry = () => {
    this.setState({ error: null });
    this.props.onRetry?.();
  };

  render() {
    if (!this.state.error) return this.props.children;
    return (
      <div
        role="alert"
        className="flex flex-col items-start gap-3 rounded-xl border border-destructive/30 bg-destructive/10 p-6 text-sm text-destructive"
      >
        <h2 className="text-base font-semibold">{this.props.title}</h2>
        <p className="max-w-prose text-muted-foreground">{this.props.message}</p>
        <Button variant="outline" onClick={this.retry}>
          <RefreshCw className="size-4" aria-hidden="true" />
          {this.props.retryLabel}
        </Button>
      </div>
    );
  }
}

/** i18n-aware wrapper used around each pane's content. */
export function PaneErrorBoundary({
  children,
  onRetry,
}: {
  children: ReactNode;
  onRetry?: () => void;
}) {
  const t = useT();
  return (
    <ErrorBoundary
      title={t('errorBoundary.title')}
      message={t('errorBoundary.message')}
      retryLabel={t('common.retry')}
      onRetry={onRetry}
    >
      {children}
    </ErrorBoundary>
  );
}
