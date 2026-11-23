import type { ReactNode } from 'react';
import { SettingsIcon } from './SettingsIcon';
import type { IconName, Tone } from './types';

export interface InstallInvitationProps {
  symbol: IconName;
  color?: Tone;
  title: string;
  message: string;
  iconLabelKey?: string;
  actions?: ReactNode;
  footer?: ReactNode;
}

export function InstallInvitation({
  symbol,
  color = 'neutral',
  title,
  message,
  iconLabelKey,
  actions,
  footer,
}: InstallInvitationProps) {
  return (
    <div className="install-invitation">
      <SettingsIcon symbol={symbol} color={color} size={56} labelKey={iconLabelKey} />
      <h2>{title}</h2>
      <p>{message}</p>
      {actions && <div className="install-invitation-actions">{actions}</div>}
      {footer && <p className="install-invitation-footer">{footer}</p>}
    </div>
  );
}
