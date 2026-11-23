import type { ReactNode } from 'react';
import { useT, type TranslateParams } from '../i18n';
import { Badge as UiBadge, type BadgeTone } from './ui/Badge';

export type { BadgeTone } from './ui/Badge';

export interface BadgeProps {
  labelKey?: string;
  label?: ReactNode;
  children?: ReactNode;
  params?: TranslateParams;
  tone?: BadgeTone;
  titleKey?: string;
  className?: string;
  dot?: boolean;
}

export function Badge({ labelKey, label, children, params, tone = 'neutral', titleKey, className, dot }: BadgeProps) {
  const t = useT();
  const content = children ?? label ?? (labelKey ? t(labelKey, params) : null);
  return (
    <UiBadge tone={tone} dot={dot} title={titleKey ? t(titleKey) : undefined} className={className}>
      {content}
    </UiBadge>
  );
}
