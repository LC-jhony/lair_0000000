import { useT, type TranslateParams } from '../i18n';

export interface BusyLabelProps {
  titleKey: string;
  busyTitleKey?: string;
  isBusy?: boolean;
  params?: TranslateParams;
}

export function BusyLabel({ titleKey, busyTitleKey, isBusy = false, params }: BusyLabelProps) {
  const t = useT();
  return (
    <span className="busy-label">
      {isBusy && <span className="spinner spinner-small" aria-hidden="true" />}
      {t(isBusy && busyTitleKey ? busyTitleKey : titleKey, params)}
    </span>
  );
}
