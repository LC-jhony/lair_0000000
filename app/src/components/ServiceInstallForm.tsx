import { Download } from 'lucide-react';
import { useT } from '../i18n';
import { Button } from './ui/ActionButton';
import { Input } from './ui/FieldInput';
import { Select } from './ui/Select';
import type { CatalogLineOption } from './types';

export interface ServiceInstallFormProps {
  name: string;
  lines: CatalogLineOption[];
  selectedLine?: string;
  onSelectLine?: (id: string) => void;
  port: number;
  onPortChange?: (port: number) => void;
  defaultPort: number;
  suggestedPort?: number;
  /** PostgreSQL changes the port after installing, so the field is read-only. */
  allowPort?: boolean;
  busy?: boolean;
  estimate?: string;
  onInstall: (line: string, port: number) => void;
}

export function ServiceInstallForm({
  name,
  lines,
  selectedLine,
  onSelectLine,
  port,
  onPortChange,
  defaultPort,
  suggestedPort,
  allowPort = true,
  busy = false,
  estimate,
  onInstall,
}: ServiceInstallFormProps) {
  const t = useT();
  const selected = selectedLine ?? lines.find((line) => line.active)?.id ?? lines[0]?.id ?? '';
  const line = lines.find((item) => item.id === selected);
  const portValid = !allowPort || (Number.isInteger(port) && port >= 1024 && port <= 65535);
  const showSuggestion =
    allowPort && suggestedPort !== undefined && suggestedPort !== defaultPort && port === suggestedPort;
  const installLabel = line?.latest
    ? t('services.install.action', { name, version: line.latest })
    : t('services.install.actionNoVersion', { name });

  return (
    <form
      className="flex flex-col gap-4"
      onSubmit={(event) => {
        event.preventDefault();
        if (portValid && selected && !busy) onInstall(selected, port);
      }}
    >
      <div className="grid gap-3 sm:grid-cols-[minmax(0,1fr)_7rem]">
        <Select
          label={t('services.install.version')}
          value={selected}
          onChange={(value) => onSelectLine?.(value)}
          disabled={busy}
          options={lines.map((item) => ({
            value: item.id,
            label: item.latest ? `${item.label} (${item.latest})` : item.label,
          }))}
        />
        <div className="flex flex-col gap-1.5">
          <span className="text-xs font-medium text-muted-foreground">{t('services.install.port')}</span>
          <Input
            type="number"
            min={1024}
            max={65535}
            value={Number.isFinite(port) ? port : ''}
            disabled={!allowPort || busy}
            invalid={!portValid}
            onChange={(event) => onPortChange?.(Number(event.target.value))}
          />
        </div>
      </div>

      {!portValid && <p className="text-xs text-destructive">{t('common.portRangeError')}</p>}
      {showSuggestion && (
        <p className="text-xs text-muted-foreground">
          {t('common.portTaken', { defaultPort, port: suggestedPort })}
        </p>
      )}

      <Button type="submit" disabled={!portValid || !selected || busy}>
        <Download className="size-4" aria-hidden="true" />
        {busy ? t('common.installing') : installLabel}
      </Button>

      <p className="text-xs text-muted-foreground">
        {estimate ? t('common.estimate', { estimate }) : t('common.estimateUnknown')}
      </p>
    </form>
  );
}
