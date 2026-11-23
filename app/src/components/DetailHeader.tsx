import { SettingsIcon } from './SettingsIcon';
import { StatusDot } from './StatusDot';
import type { IconName, Tone } from './types';

export interface DetailHeaderProps {
  title: string;
  symbol: IconName;
  color?: Tone;
  status?: string;
  running?: boolean;
  iconLabelKey?: string;
}

export function DetailHeader({
  title,
  symbol,
  color = 'neutral',
  status,
  running,
  iconLabelKey,
}: DetailHeaderProps) {
  return (
    <header className="detail-header">
      <SettingsIcon symbol={symbol} color={color} size={40} labelKey={iconLabelKey} />
      <div className="detail-header-text">
        <h2>{title}</h2>
        {status && <p>{status}</p>}
      </div>
      {running !== undefined && <StatusDot on={running} />}
    </header>
  );
}
