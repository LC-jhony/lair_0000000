import { useT } from '../i18n';
import type { TranslateParams } from '../i18n';

export interface PaneHeaderProps {
  titleKey: string;
  subtitleKey?: string;
  params?: TranslateParams;
}

export function PaneHeader({ titleKey, subtitleKey, params }: PaneHeaderProps) {
  const t = useT();
  return (
    <header className="pane-header">
      <h1>{t(titleKey, params)}</h1>
      {subtitleKey && <p>{t(subtitleKey, params)}</p>}
    </header>
  );
}
