import type { ReactNode } from 'react';
import { useT, type TranslateParams } from '../i18n';

export interface DetailSectionTitleProps {
  titleKey: string;
  params?: TranslateParams;
  trailing?: ReactNode;
}

export function DetailSectionTitle({ titleKey, params, trailing }: DetailSectionTitleProps) {
  const t = useT();
  return (
    <div className="detail-section-title">
      <h3>{t(titleKey, params)}</h3>
      {trailing}
    </div>
  );
}
