import { useT } from '../i18n';
import { Button } from './ui/ActionButton';
import { normalizedProgress, type ComponentJob } from './types';

export interface ComponentJobProgressProps {
  title: string;
  job: ComponentJob;
  onShowLog?: () => void;
  onCancel?: () => void;
}

export function ComponentJobProgress({ title, job, onShowLog, onCancel }: ComponentJobProgressProps) {
  const t = useT();
  const progress = normalizedProgress(job);
  const determinate = progress !== null && job.phase !== 'installing';
  const text =
    job.phase === 'installing'
      ? t('common.installingProgress')
      : progress === null
        ? t('common.downloading')
        : t('common.downloadingPercent', { percent: Math.round(progress * 100) });

  return (
    <div data-testid="job-progress" className="rounded-xl border border-border bg-card p-4 shadow-sm">
      <div className="flex items-center justify-between gap-3">
        <span className="truncate text-sm font-medium">{title}</span>
        <div className="flex shrink-0 items-center gap-1">
          {onShowLog && (
            <Button variant="ghost" size="sm" onClick={onShowLog}>{t('common.showLog')}</Button>
          )}
          {onCancel && (
            <Button variant="outline" size="sm" onClick={onCancel}>{t('common.cancel')}</Button>
          )}
        </div>
      </div>
      <div className="mt-3 flex items-center gap-3">
        {determinate ? (
          <progress value={progress ?? 0} max={1} aria-label={text} className="h-2 flex-1 accent-primary" />
        ) : (
          <span
            className="spinner size-4 animate-spin rounded-full border-2 border-muted border-t-primary"
            aria-label={text}
            role="status"
          />
        )}
        <span className="text-xs text-muted-foreground">{text}</span>
      </div>
    </div>
  );
}
