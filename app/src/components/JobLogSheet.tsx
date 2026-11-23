import { useEffect, useState } from 'react';
import { useDialogFocus } from '../hooks/use-dialog-focus';
import { useT } from '../i18n';

export interface JobLogSheetProps {
  title: string;
  running: boolean;
  load: () => Promise<string>;
  onClose: () => void;
  refreshMs?: number;
}

export function JobLogSheet({
  title,
  running,
  load,
  onClose,
  refreshMs = 2000,
}: JobLogSheetProps) {
  const t = useT();
  const dialogRef = useDialogFocus<HTMLElement>(true, onClose);
  const [log, setLog] = useState('');

  useEffect(() => {
    let cancelled = false;
    const refresh = () => {
      void load()
        .then((text) => {
          if (!cancelled) setLog(text);
        })
        .catch(() => {
          if (!cancelled) setLog('');
        });
    };
    refresh();
    if (!running) return () => { cancelled = true; };
    const timer = setInterval(refresh, refreshMs);
    return () => {
      cancelled = true;
      clearInterval(timer);
    };
  }, [load, running, refreshMs]);

  return (
    <div className="sheet-backdrop">
      <section
        ref={dialogRef}
        className="sheet"
        role="dialog"
        aria-modal="true"
        aria-label={title}
      >
        <header className="sheet-head">
          <h2>{title}</h2>
          <button type="button" className="button button-small" onClick={onClose}>
            {t('common.close')}
          </button>
        </header>
        <pre className="sheet-log">{log || t('common.loading')}</pre>
      </section>
    </div>
  );
}
