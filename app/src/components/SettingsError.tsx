import { CircleAlert } from 'lucide-react';
import { useT } from '../i18n';
import { Button } from './ui/ActionButton';

export interface SettingsErrorProps {
  message: string;
  onDismiss?: () => void;
  dismissLabelKey?: string;
}

export function SettingsError({
  message,
  onDismiss,
  dismissLabelKey = 'common.close',
}: SettingsErrorProps) {
  const t = useT();
  if (!message) return null;
  return (
    <div role="alert" className="fixed bottom-4 right-4 z-50 flex max-w-md items-start gap-3 rounded-xl border border-destructive/30 bg-destructive/10 p-4 text-sm text-destructive shadow-lg backdrop-blur">
      <CircleAlert className="mt-0.5 size-4 shrink-0" aria-hidden="true" />
      <span className="flex-1">{message}</span>
      {onDismiss && (
        <Button variant="ghost" size="sm" onClick={onDismiss} className="-my-1 text-destructive hover:bg-destructive/10">
          {t(dismissLabelKey)}
        </Button>
      )}
    </div>
  );
}
