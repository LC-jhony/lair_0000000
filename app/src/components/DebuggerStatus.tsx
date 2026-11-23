import { Bug } from 'lucide-react';
import type { DebuggerResult, DebuggerOp } from '../api';
import { useT } from '../i18n';
import { Badge } from './Badge';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from './ui/Card';
import { Switch } from './ui/Switch';
import { StatusDot } from './StatusDot';

export interface DebuggerStatusProps {
  result: DebuggerResult | null;
  busy?: boolean;
  onToggle: (op: DebuggerOp) => void;
}

/** Live Xdebug/PhpStorm detection state and its two master switches. */
export function DebuggerStatus({ result, busy = false, onToggle }: DebuggerStatusProps) {
  const t = useT();
  const detect = Boolean(result?.detect);
  const breakpoints = Boolean(result?.breakpoints);
  const running = detect || breakpoints;

  return (
    <Card>
      <CardHeader className="flex-row items-start justify-between space-y-0">
        <div className="flex items-start gap-3">
          <span className="grid size-10 shrink-0 place-items-center rounded-xl bg-primary/10 text-primary">
            <Bug className="size-5" aria-hidden="true" />
          </span>
          <div>
            <CardTitle className="flex items-center gap-2">
              {t('pane.debugger')}
              <StatusDot on={running} />
            </CardTitle>
            <CardDescription>{t('debugger.status', { mode: result?.mode || '—' })}</CardDescription>
          </div>
        </div>
        <div className="flex flex-wrap justify-end gap-1.5">
          <Badge tone="indigo">{t('debugger.port', { port: result?.port ?? 9003 })}</Badge>
          <Badge tone={detect ? 'green' : 'neutral'} dot>{t('debugger.detect')}</Badge>
          <Badge tone={breakpoints ? 'green' : 'neutral'} dot>{t('debugger.breakpoints')}</Badge>
        </div>
      </CardHeader>
      <CardContent className="flex flex-col gap-4">
        <div className="grid gap-4 rounded-xl border border-border bg-muted/30 p-4 md:grid-cols-2">
          <Switch
            checked={detect}
            onChange={(checked) => onToggle(checked ? 'detect-on' : 'detect-off')}
            label={t('debugger.detect')}
            description={t('debugger.detectHelp')}
            disabled={busy}
          />
          <Switch
            checked={breakpoints}
            onChange={(checked) => onToggle(checked ? 'breakpoints-on' : 'breakpoints-off')}
            label={t('debugger.breakpoints')}
            description={t('debugger.breakpointsHelp')}
            disabled={busy}
          />
        </div>

        <div className="flex flex-col gap-2">
          <p className="text-xs font-medium text-muted-foreground">{t('debugger.availability')}</p>
          <div className="flex flex-wrap gap-1.5">
            {(result?.versions ?? []).length === 0 && <span className="text-xs text-muted-foreground">—</span>}
            {result?.versions?.map((version) => (
              <Badge key={version.version} tone={version.available ? 'green' : 'neutral'} dot>
                {t('debugger.phpVersion', { version: version.version ?? '—' })}
              </Badge>
            ))}
          </div>
          {(result?.domains?.length ?? 0) > 0 && (
            <div className="flex flex-wrap gap-1.5">
              {result?.domains?.map((domain) => (
                <Badge key={domain} tone="blue">{domain}</Badge>
              ))}
            </div>
          )}
        </div>
      </CardContent>
    </Card>
  );
}
