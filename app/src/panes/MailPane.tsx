import { useCallback, useEffect, useRef, useState } from 'react';
import { Inbox, Mail, Play, Square, Trash2 } from 'lucide-react';
import type { MailMessage, MailPayload, MailResult } from '../api';
import {
  Badge,
  Button,
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
  CodeBlock,
  Dialog,
  EmptyState,
  MailMessageView,
  StatCard,
  StatusDot,
} from '../components';
import { useT } from '../i18n';
import type { PaneProps } from './types';

/** Local SMTP catcher: status, captured messages and their raw representations. */
export function MailPane({ state }: PaneProps) {
  const t = useT();
  const [result, setResult] = useState<MailResult | null>(null);
  const [selected, setSelected] = useState<MailMessage | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [confirmClear, setConfirmClear] = useState(false);
  const [openDialog, setOpenDialog] = useState(false);
  const aliveRef = useRef(true);

  const load = useCallback(async () => {
    if (!state.client) return;
    try {
      const [status, list] = await Promise.all([
        state.client.mail({ op: 'status' }),
        state.client.mail({ op: 'list' }),
      ]);
      if (!aliveRef.current) return;
      const merged: MailResult = {
        ...status,
        ...list,
        count: list.messages?.length ?? status.count ?? 0,
      };
      setResult(merged);
      if (selected) {
        const refreshed = (merged.messages ?? []).find((message) => message.id === selected.id);
        if (refreshed) setSelected(refreshed);
      }
    } catch (cause) {
      if (aliveRef.current) setError(cause instanceof Error ? cause.message : String(cause));
    }
  }, [state.client, selected]);

  useEffect(() => {
    aliveRef.current = true;
    void load();
    return () => { aliveRef.current = false; };
  }, [state.client]); // eslint-disable-line react-hooks/exhaustive-deps

  const run = async (payload: MailPayload) => {
    if (!state.client) return;
    setBusy(true);
    setError('');
    try {
      const next = await state.client.mail(payload);
      if (!aliveRef.current) return;
      // Daemon responses are partial: keep the inbox from the previous snapshot
      // when an operation only reports status, and clear it explicitly on `clear`.
      setResult((previous) => {
        const merged = { ...(previous ?? {}), ...next };
        if (payload.op === 'clear') return { ...merged, messages: [], count: 0 };
        if (next.messages) return { ...merged, count: next.messages.length };
        return { ...merged, messages: previous?.messages, count: next.count ?? previous?.count ?? 0 };
      });
      if (payload.op === 'message') setSelected(next.message ?? null);
      if (payload.op === 'clear') {
        setSelected(null);
        setConfirmClear(false);
        setOpenDialog(false);
      }
      await state.refresh();
    } catch (cause) {
      if (aliveRef.current) setError(cause instanceof Error ? cause.message : String(cause));
    } finally {
      if (aliveRef.current) setBusy(false);
    }
  };

  const messages = result?.messages ?? [];
  const busyAll = busy || !!state.busy;
  const port = result?.port ?? 2525;
  const domain = state.status?.web?.domain ?? 'test';
  const mailEnv = [
    'MAIL_MAILER=smtp',
    'MAIL_HOST=127.0.0.1',
    `MAIL_PORT=${port}`,
    'MAIL_USERNAME=lair',
    'MAIL_PASSWORD=null',
    'MAIL_SCHEME=null',
    `MAIL_FROM_ADDRESS=no-reply@${domain}`,
  ].join('\n');

  return (
    <div className="flex flex-col gap-5">
      <Card>
        <CardHeader className="flex-row items-start justify-between space-y-0">
          <div className="flex items-start gap-3">
            <span className="grid size-10 shrink-0 place-items-center rounded-xl bg-primary/10 text-primary">
              <Mail className="size-5" aria-hidden="true" />
            </span>
            <div>
              <CardTitle className="flex items-center gap-2">
                SMTP
                <StatusDot on={Boolean(result?.running)} />
                <Badge tone={result?.running ? 'green' : 'neutral'} dot>
                  {result?.running ? t('common.running') : t('common.stopped')}
                </Badge>
              </CardTitle>
              <CardDescription>{t('pane.mail.subtitle')}</CardDescription>
            </div>
          </div>
          <div className="flex flex-wrap justify-end gap-2">
            {result?.running ? (
              <Button variant="outline" size="sm" disabled={busyAll} onClick={() => void run({ op: 'stop' })}>
                <Square className="size-3.5" aria-hidden="true" />
                {t('common.stop')}
              </Button>
            ) : (
              <Button size="sm" disabled={busyAll} onClick={() => void run({ op: 'start' })}>
                <Play className="size-3.5" aria-hidden="true" />
                {t('common.start')}
              </Button>
            )}
            <Button
              variant="outline"
              size="sm"
              disabled={busyAll || messages.length === 0}
              onClick={() => setOpenDialog(true)}
            >
              <Mail className="size-3.5" aria-hidden="true" />
              {t('mail.open')}
            </Button>
          </div>
        </CardHeader>
        <CardContent className="flex flex-col gap-4">
          <div className="grid gap-3 sm:grid-cols-3">
            <StatCard label={t('mail.port')} value={String(port)} icon={<Mail />} />
            <StatCard label={t('mail.count')} value={String(messages.length)} icon={<Inbox />} />
            <StatCard label={t('mail.environment')} value={result?.env || '—'} />
          </div>
          <CodeBlock text={mailEnv} />
          <p className="text-xs text-muted-foreground">{t('mail.inboxHint')}</p>
          {result?.output && <p className="text-xs text-muted-foreground">{result.output}</p>}
          {error && <p role="alert" className="text-xs text-red-600 dark:text-red-400">{error}</p>}
        </CardContent>
      </Card>

      <div className="grid gap-4 lg:grid-cols-[minmax(0,22rem)_minmax(0,1fr)]">
        <Card>
          <CardHeader className="flex-row items-center justify-between space-y-0">
            <CardTitle>{t('mail.inbox')}</CardTitle>
            <Button
              variant="ghost"
              size="sm"
              disabled={busyAll || messages.length === 0}
              onClick={() => setConfirmClear(true)}
            >
              <Trash2 className="size-3.5" aria-hidden="true" />
              {t('mail.clearAll')}
            </Button>
          </CardHeader>
          <CardContent>
            {messages.length === 0 ? (
              <EmptyState icon={<Inbox />} title={t('mail.empty')} description={t('mail.emptyHelp')} />
            ) : (
              <ul className="flex flex-col gap-2">
                {messages.map((message) => (
                  <li key={message.id}>
                    <button
                      type="button"
                      disabled={busyAll}
                      onClick={() => void run({ op: 'message', id: message.id })}
                      aria-current={selected?.id === message.id ? 'true' : undefined}
                      className={`flex w-full flex-col gap-0.5 rounded-lg border px-3 py-2 text-left transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring ${
                        selected?.id === message.id
                          ? 'border-primary/50 bg-primary/10'
                          : 'border-border bg-muted/30 hover:bg-accent'
                      }`}
                    >
                      <span className="truncate text-xs font-medium">{message.subject || t('mail.noSubject')}</span>
                      <span className="truncate text-[11px] text-muted-foreground">{message.from || '—'}</span>
                      <span className="truncate text-[11px] text-muted-foreground">{message.date || '—'}</span>
                    </button>
                  </li>
                ))}
              </ul>
            )}
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>{t('mail.detail')}</CardTitle>
            <CardDescription>{t('mail.detailHelp')}</CardDescription>
          </CardHeader>
          <CardContent><MailMessageView message={selected} /></CardContent>
        </Card>
      </div>

      <Dialog
        open={openDialog}
        onClose={() => setOpenDialog(false)}
        title={t('mail.open')}
        description={t('mail.detailHelp')}
        size="lg"
      >
        <MailMessageView message={selected ?? messages[0] ?? null} />
      </Dialog>

      <Dialog
        open={confirmClear}
        onClose={() => setConfirmClear(false)}
        title={t('mail.clear.title')}
        description={t('mail.clear.message')}
        size="sm"
        footer={
          <>
            <Button variant="ghost" onClick={() => setConfirmClear(false)}>{t('common.cancel')}</Button>
            <Button variant="danger" disabled={busyAll} onClick={() => void run({ op: 'clear' })}>
              <Trash2 className="size-3.5" aria-hidden="true" />
              {t('common.clear')}
            </Button>
          </>
        }
      />
    </div>
  );
}
