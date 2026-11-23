import { CircleAlert } from 'lucide-react';
import { useT } from '../i18n';
import { Button } from './ui/ActionButton';
import type { ComponentJob } from './types';

export interface ComponentJobFailureProps {
  name: string;
  job: ComponentJob;
  onShowLog?: () => void;
  onRetry?: () => void;
  onDismiss?: () => void;
}

export function ComponentJobFailure({
  name,
  job,
  onShowLog,
  onRetry,
  onDismiss,
}: ComponentJobFailureProps) {
  const t = useT();
  const error = job.error?.trim() || t('common.downloadFailed');

  return (
    <div data-testid="job-failure" className="rounded-xl border border-destructive/30 bg-destructive/5 p-4">
      <div className="flex items-center justify-between gap-3">
        <span className="flex items-center gap-2 text-sm font-medium">
          <CircleAlert className="size-4 text-destructive" aria-hidden="true" />
          {name}
        </span>
        <div className="flex shrink-0 items-center gap-1">
          {onShowLog && <Button variant="ghost" size="sm" onClick={onShowLog}>{t('common.showLog')}</Button>}
          {onRetry && <Button variant="outline" size="sm" onClick={onRetry}>{t('common.retry')}</Button>}
          {onDismiss && <Button variant="outline" size="sm" onClick={onDismiss}>{t('common.dismiss')}</Button>}
        </div>
      </div>
      <p className="mt-2 truncate text-xs text-destructive" title={error}>{error}</p>
    </div>
  );
}
