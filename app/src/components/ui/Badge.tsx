import type { HTMLAttributes } from 'react';
import { cn } from '@/lib/utils';

export type BadgeTone =
  | 'neutral'
  | 'gray'
  | 'blue'
  | 'indigo'
  | 'green'
  | 'orange'
  | 'brown'
  | 'cyan'
  | 'pink'
  | 'red'
  | 'purple'
  | 'teal';

const tones: Record<BadgeTone, string> = {
  neutral: 'bg-muted text-muted-foreground',
  gray: 'bg-muted text-muted-foreground',
  blue: 'bg-blue-500/10 text-blue-600 dark:text-blue-400',
  indigo: 'bg-primary/10 text-primary',
  green: 'bg-emerald-500/10 text-emerald-600 dark:text-emerald-400',
  orange: 'bg-orange-500/10 text-orange-600 dark:text-orange-400',
  brown: 'bg-amber-800/10 text-amber-800 dark:text-amber-300',
  cyan: 'bg-cyan-500/10 text-cyan-600 dark:text-cyan-400',
  pink: 'bg-pink-500/10 text-pink-600 dark:text-pink-400',
  red: 'bg-destructive/10 text-destructive',
  purple: 'bg-purple-500/10 text-purple-600 dark:text-purple-400',
  teal: 'bg-teal-500/10 text-teal-600 dark:text-teal-400',
};

export interface BadgeProps extends HTMLAttributes<HTMLSpanElement> {
  tone?: BadgeTone;
  dot?: boolean;
}

export function Badge({ tone = 'neutral', dot = false, className, children, ...props }: BadgeProps) {
  return (
    <span
      className={cn(
        'inline-flex items-center gap-1.5 rounded-full px-2 py-0.5 text-[11px] font-medium',
        tones[tone],
        className,
      )}
      {...props}
    >
      {dot && <span className="size-1.5 rounded-full bg-current" aria-hidden="true" />}
      {children}
    </span>
  );
}
