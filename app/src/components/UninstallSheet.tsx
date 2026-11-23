import { useState } from 'react';
import { Trash2 } from 'lucide-react';
import { useT } from '../i18n';
import { Badge } from './Badge';
import { CodeBlock } from './CodeBlock';
import { Dialog } from './ui/Dialog';
import { Switch } from './ui/Switch';
import { Button } from './ui/ActionButton';

export interface UninstallSheetProps {
  open: boolean;
  onClose: () => void;
  home?: string;
  onUninstall?: (deleteData: boolean) => void;
}

/** Guidance sheet for removing Lair. The destructive step is always a CLI command. */
export function UninstallSheet({ open, onClose, home, onUninstall }: UninstallSheetProps) {
  const t = useT();
  const [deleteData, setDeleteData] = useState(false);

  return (
    <Dialog
      open={open}
      onClose={onClose}
      title={t('uninstall.title')}
      description={t('uninstall.description')}
      size="md"
      footer={
        <>
          <Button variant="outline" onClick={onClose}>{t('common.close')}</Button>
          {onUninstall && (
            <Button variant="danger" onClick={() => { onUninstall(deleteData); onClose(); }}>
              <Trash2 className="size-3.5" aria-hidden="true" />
              {t('uninstall.action')}
            </Button>
          )}
        </>
      }
    >
      <div className="flex flex-col gap-4">
        <div className="rounded-lg border border-border bg-muted/30 p-4">
          <Switch
            checked={deleteData}
            onChange={setDeleteData}
            label={t('uninstall.deleteData')}
            description={t('uninstall.deleteDataHelp')}
          />
        </div>
        <div className="flex flex-col gap-1.5">
          <p className="text-xs font-medium text-muted-foreground">{t('uninstall.command')}</p>
          <CodeBlock text={deleteData ? 'lair uninstall --delete-data' : 'lair uninstall --keep-data'} />
        </div>
        {home && <p className="truncate text-xs text-muted-foreground">{home}</p>}
        <div className="flex flex-wrap items-center gap-2">
          <Badge tone="orange" dot><Trash2 className="size-3" aria-hidden="true" />{t('uninstall.manual')}</Badge>
        </div>
        <p className="text-xs text-muted-foreground">{t('uninstall.note')}</p>
      </div>
    </Dialog>
  );
}
