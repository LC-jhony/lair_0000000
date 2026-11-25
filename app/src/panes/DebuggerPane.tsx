import { useCallback, useEffect, useRef, useState } from 'react';
import { Cookie, PlugZap, RefreshCw } from 'lucide-react';
import type { DebuggerOp, DebuggerResult } from '../api';
import {
  Badge,
  Button,
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
  CodeBlock,
  DebuggerStatus,
} from '../components';
import { useT } from '../i18n';
import type { PaneProps } from './types';

/** Xdebug/PhpStorm session console. */
export function DebuggerPane({ state }: PaneProps) {
  const t = useT();
  const [result, setResult] = useState<DebuggerResult | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const aliveRef = useRef(true);

  const load = useCallback(async () => {
    if (!state.client) return;
    try {
      const next = await state.client.debugger({ op: 'status' });
      if (aliveRef.current) setResult(next);
    } catch (cause) {
      if (aliveRef.current) setError(cause instanceof Error ? cause.message : String(cause));
    }
  }, [state.client]);

  useEffect(() => {
    aliveRef.current = true;
    void load();
    return () => { aliveRef.current = false; };
  }, [load]);

  const toggle = async (op: DebuggerOp) => {
    if (!state.client) return;
    setBusy(true);
    setError('');
    try {
      const next = await state.client.debugger({ op });
      if (!aliveRef.current) return;
      setResult(next);
      await state.refresh();
    } catch (cause) {
      if (aliveRef.current) setError(cause instanceof Error ? cause.message : String(cause));
    } finally {
      if (aliveRef.current) setBusy(false);
    }
  };

  const port = result?.port ?? 9003;
  const domain = result?.domains?.[0] ?? state.status?.sites?.[0]?.name ?? 'demo.test';
  const env = [
    'XDEBUG_MODE=debug,develop',
    `XDEBUG_CONFIG="client_host=127.0.0.1 client_port=${port}"`,
    `PHP_IDE_CONFIG="serverName=${domain}"`,
  ].join('\n');
  const cookie = 'XDEBUG_TRIGGER=1; XDEBUG_SESSION=PHPSTORM';

  return (
    <div className="flex flex-col gap-5">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div className="flex flex-wrap gap-2">
          <Badge tone="indigo" dot>{t('debugger.port', { port })}</Badge>
          <Badge tone="neutral">{t('debugger.mode', { mode: result?.mode || '—' })}</Badge>
        </div>
        <Button variant="ghost" size="sm" disabled={busy || !!state.busy} onClick={() => void load()}>
          <RefreshCw className="size-3.5" aria-hidden="true" />
          {t('dash.refresh')}
        </Button>
      </div>

      <DebuggerStatus result={result} busy={busy || !!state.busy} onToggle={(op) => void toggle(op)} />

      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <PlugZap className="size-4 text-primary" aria-hidden="true" />
            {t('debugger.guide.title')}
          </CardTitle>
          <CardDescription>{t('debugger.guide.help')}</CardDescription>
        </CardHeader>
        <CardContent className="flex flex-col gap-4">
          <div className="flex flex-col gap-1.5">
            <p className="text-xs font-medium text-muted-foreground">{t('debugger.guide.env')}</p>
            <CodeBlock text={env} />
          </div>
          <div className="flex flex-col gap-1.5">
            <p className="flex items-center gap-1.5 text-xs font-medium text-muted-foreground">
              <Cookie className="size-3.5" aria-hidden="true" />
              {t('debugger.guide.cookie')}
            </p>
            <CodeBlock text={cookie} />
          </div>
          <p className="text-xs text-muted-foreground">{t('debugger.guide.ide')}</p>
          {error && <p role="alert" className="text-xs text-red-600 dark:text-red-400">{error}</p>}
        </CardContent>
      </Card>
    </div>
  );
}
