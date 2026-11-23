import { useT } from '../i18n';
import { Icon } from './Icon';
import type { IconName, Tone } from './types';

export interface SettingsIconProps {
  symbol: IconName;
  color?: Tone;
  size?: number;
  labelKey?: string;
}

export function SettingsIcon({ symbol, color = 'indigo', size = 26, labelKey }: SettingsIconProps) {
  const t = useT();
  const label = labelKey ? t(labelKey) : undefined;
  return (
    <span
      className="settings-icon"
      data-tone={color}
      style={{ width: size, height: size }}
      role={label ? 'img' : undefined}
      aria-label={label}
      aria-hidden={label ? undefined : true}
    >
      <Icon name={symbol} size={Math.round(size * 0.62)} />
    </span>
  );
}
