import { useT } from '../i18n';
import { Button } from './ui/ActionButton';

export interface CheckForUpdatesLineProps {
  checkedAt?: string | null;
  error?: string;
  disabled?: boolean;
  checking?: boolean;
  onCheck?: () => void;
}

export function CheckForUpdatesLine({
  checkedAt,
  error,
  disabled = false,
  checking = false,
  onCheck,
}: CheckForUpdatesLineProps) {
  const t = useT();
  const label = checking
    ? t('common.checking')
    : checkedAt
      ? t('common.checkedAgo', { time: checkedAt })
      : t('common.notChecked');

  return (
    <div className="flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
      <span>{label}</span>
      <span aria-hidden="true">·</span>
      <Button variant="ghost" size="sm" className="h-7 px-2" onClick={onCheck} disabled={disabled || checking}>
        {t('common.checkNow')}
      </Button>
      {error && <span className="text-destructive">{error}</span>}
    </div>
  );
}
