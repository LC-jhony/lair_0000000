import { cn } from '@/lib/utils';

export function Progress({
  value,
  max = 100,
  className,
  label,
  tone,
}: {
  value: number;
  max?: number;
  className?: string;
  label?: string;
  /** Optional colour by state: red < 50%, orange < 100%, green at 100%. */
  tone?: 'danger' | 'warning' | 'success';
}) {
  const percent = max > 0 ? Math.min(100, Math.max(0, (value / max) * 100)) : 0;
  const fill = tone === 'danger' ? 'bg-destructive'
    : tone === 'warning' ? 'bg-amber-500'
      : tone === 'success' ? 'bg-emerald-500'
        : 'bg-primary';
  return (
    <div
      role="progressbar"
      aria-valuenow={Math.round(value)}
      aria-valuemin={0}
      aria-valuemax={max}
      aria-label={label}
      className={cn('h-2 w-full overflow-hidden rounded-full bg-muted', className)}
    >
      <div className={cn('h-full rounded-full transition-[width,background-color] duration-500', fill)} style={{ width: `${percent}%` }} />
    </div>
  );
}
