import { useT } from '../i18n';
import { cn } from '@/lib/utils';

export interface StatusDotProps {
  on: boolean;
  labelKey?: string;
  className?: string;
}

export function StatusDot({ on, labelKey, className }: StatusDotProps) {
  const t = useT();
  const label = labelKey
    ? t(labelKey)
    : on
      ? t('common.active')
      : t('common.stopped');
  return (
    <span
      role="img"
      aria-label={label}
      data-on={on}
      className={cn('inline-flex size-2.5 shrink-0 rounded-full', on ? 'bg-emerald-500' : 'bg-muted-foreground/40', className)}
    />
  );
}
