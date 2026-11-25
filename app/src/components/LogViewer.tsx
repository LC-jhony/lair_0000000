import { useDialogFocus } from '../hooks/use-dialog-focus';
import { useT } from '../i18n';

export interface LogViewerProps {
  title: string;
  text: string;
  onClose: () => void;
}

/** Read-only log panel, equivalent to `lair logs`. */
export function LogViewer({ title, text, onClose }: LogViewerProps) {
  const t = useT();
  const dialogRef = useDialogFocus<HTMLDivElement>(true, onClose);
  return (
    <div className="dialog-backdrop" role="presentation" onMouseDown={onClose}>
      <div
        ref={dialogRef}
        className="dialog dialog-wide"
        role="dialog"
        aria-modal="true"
        aria-label={title}
        onMouseDown={(event) => event.stopPropagation()}
      >
        <div className="service-head">
          <h2 className="card-title">{title}</h2>
          <button className="button button-small" type="button" onClick={onClose}>
            {t('common.close')}
          </button>
        </div>
        <pre className="log-view">{text || t('dash.noLog')}</pre>
      </div>
    </div>
  );
}
