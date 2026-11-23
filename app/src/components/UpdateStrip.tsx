import { ArrowUpRight } from 'lucide-react';
import { useT } from '../i18n';
import { Button } from './ui/ActionButton';

export interface UpdateStripProps {
  count: number;
  onUpdateAll?: () => void;
}

export function UpdateStrip({ count, onUpdateAll }: UpdateStripProps) {
  const t = useT();
  if (count <= 0) return null;
  return (
    <div role="status" className="flex items-center justify-between gap-3 rounded-xl border border-primary/20 bg-primary/5 px-4 py-3 text-sm">
      <span>{t('common.updatesAvailable', { count })}</span>
      {onUpdateAll && (
        <Button size="sm" onClick={onUpdateAll}>
          <ArrowUpRight className="size-3.5" aria-hidden="true" />
          {t('common.installAll')}
        </Button>
      )}
    </div>
  );
}
