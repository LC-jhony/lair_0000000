import { useEffect, useRef, useState, type ReactNode } from 'react';
import { Code2, Network, ServerCog, ShieldAlert } from 'lucide-react';
import {
  Badge,
  Button,
  CodeBlock,
  Dialog,
  Input,
  Select,
  Switch,
} from '../components';
import { useT } from '../i18n';
import type { WebConfigPayload, WebOp } from '../api/models';
import type { LairController } from '../useLair';

/** Defaults of the embedded server; privileged ports need an explicit grant. */
export const DEFAULT_HTTP_PORT = 80;
export const DEFAULT_HTTPS_PORT = 443;
const UNPRIVILEGED_HTTP_PORT = 8080;
const UNPRIVILEGED_HTTPS_PORT = 8443;
const PERMISSION_ERROR = /permission denied|cap_net_bind_service|operation not permitted|not permitted|privileg/i;

function SectionHeading({ icon, children }: { icon: ReactNode; children: ReactNode }) {
  return (
    <div className="flex items-center gap-2 px-0.5 pt-0.5">
      <span className="text-primary">{icon}</span>
      <span className="text-[11px] font-semibold uppercase tracking-wide text-muted-foreground">
        {children}
      </span>
    </div>
  );
}

/**
 * Lair configuration: the settings Valet Deck exposes in several panes,
 * concentrated in one modal and backed by the same daemon actions. It never
 * runs Valet, nginx or sudo; DNS and CA trust are read-only here.
 */
export function LairConfigDialog({
  state,
  open,
  onClose,
}: {
  state: LairController;
  open: boolean;
  onClose: () => void;
}) {
  const t = useT();
  const web = state.status?.web;
  const dns = state.status?.dns;
  const ca = state.status?.ca;
  const defaults = state.status?.defaults ?? {};
  const [draft, setDraft] = useState<WebConfigPayload>({});
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  const [dirs, setDirs] = useState<string[]>([]);
  const aliveRef = useRef(true);

  const bindOptions = [
    { value: '127.0.0.1', label: t('web.bind.localhost') },
    { value: '0.0.0.0', label: t('web.bind.all') },
  ];

  useEffect(() => {
    aliveRef.current = true;
    return () => {
      aliveRef.current = false;
    };
  }, []);

  // Polling replaces `web` every few seconds; reset the draft only when a real
  // value changes (for example after applying or after a TUI/CLI change).
  const webSignature = web ? JSON.stringify(web) : '';
  useEffect(() => {
    if (!web || !state.client) return;
    setDraft({
      http_port: web.http_port ?? DEFAULT_HTTP_PORT,
      https_port: web.https_port ?? DEFAULT_HTTPS_PORT,
      https: web.https ?? true,
      redirect_http: web.redirect_http ?? true,
      bind: web.bind ?? '127.0.0.1',
      domain: web.domain ?? 'test',
    });
  }, [webSignature, state.client]);

  useEffect(() => {
    if (!open || !state.client) return;
    let alive = true;
    void state.client.projectDirs()
      .then((result) => { if (alive) setDirs(result.dirs ?? []); })
      .catch(() => { if (alive) setDirs([]); });
    return () => { alive = false; };
  }, [open, state.client]);

  const run = async (op: WebOp, payload: WebConfigPayload = {}) => {
    if (!state.client) return;
    setBusy(true);
    setError('');
    try {
      await state.client.web(op, payload);
      if (!aliveRef.current) return;
      await state.refresh();
    } catch (cause) {
      if (aliveRef.current) setError(cause instanceof Error ? cause.message : String(cause));
    } finally {
      if (aliveRef.current) setBusy(false);
    }
  };

  const apply = () => {
    const ports = [draft.http_port ?? DEFAULT_HTTP_PORT, draft.https_port ?? DEFAULT_HTTPS_PORT];
    if (ports.some((p) => !p || p < 1 || p > 65535) || ports[0] === ports[1]) {
      setError(t('web.invalidPorts'));
      return;
    }
    void run('configure', draft);
  };

  const busyAll = busy || !!state.busy;
  const privilegedError = PERMISSION_ERROR.test(web?.error ?? '');

  const useUnprivilegedPorts = () => {
    void run('configure', {
      ...draft,
      http_port: UNPRIVILEGED_HTTP_PORT,
      https_port: UNPRIVILEGED_HTTPS_PORT,
    });
  };

  const phpOptions = (state.status?.php ?? [])
    .map((info) => String(info.minor ?? info.version ?? '').trim())
    .filter(Boolean);
  const nodeOptions = (state.status?.node ?? []).map(String).filter(Boolean);
  const home = state.status?.home ?? '<LAIR_HOME>';
  const repair = [
    `sudo ${home}/setup-privileged.sh`,
    `sudo ${home}/trust-ca.sh --add`,
    `${home}/trust-ca-user.sh --add`,
  ].join('\n');

  return (
    <Dialog
      open={open}
      onClose={onClose}
      title={t('lair.config.title')}
      description={t('lair.config.subtitle')}
      size="xl"
      scrollable={false}
      contentClassName="p-4"
      footer={
        <>
          <Button variant="ghost" onClick={onClose} disabled={busyAll}>{t('common.cancel')}</Button>
          <Button onClick={apply} disabled={busyAll}>{t('web.apply')}</Button>
        </>
      }
    >
      <div className="flex flex-col gap-3">
        <div className="grid gap-3 md:grid-cols-2">
          <div className="flex flex-col gap-3">
            <SectionHeading icon={<ServerCog className="size-3.5" aria-hidden="true" />}>
              {t('lair.config.section.server')}
            </SectionHeading>
        <div className="rounded-xl border border-border bg-muted/30 p-3">
          <div className="flex items-center justify-between gap-2">
            <span className="text-sm font-medium">{t('lair.config.statusTitle')}</span>
            <Badge tone={web?.error ? 'orange' : 'green'} dot>
              {web?.error ? t('common.notAvailable') : t('web.active')}
            </Badge>
          </div>
          <p className="mt-1 text-xs text-muted-foreground">{t('lair.config.statusHelp')}</p>
          {web?.warning && <p role="status" className="mt-2 text-xs text-muted-foreground">{web.warning}</p>}
          {web?.error && <p role="alert" className="mt-2 text-xs text-red-600 dark:text-red-400">{web.error}</p>}
          {privilegedError && (
            <div className="mt-3 flex flex-col gap-2 rounded-lg border border-amber-500/40 bg-amber-500/10 p-3 sm:flex-row sm:items-center sm:justify-between">
              <div className="flex items-start gap-2">
                <ShieldAlert className="mt-0.5 size-4 shrink-0 text-amber-600 dark:text-amber-400" aria-hidden="true" />
                <div>
                  <p className="text-xs font-medium">{t('web.privileges.title')}</p>
                  <p className="text-xs text-muted-foreground">{t('web.privileges.help')}</p>
                </div>
              </div>
              <Button variant="outline" size="sm" disabled={busyAll} onClick={useUnprivilegedPorts}>
                {t('web.privileges.action')}
              </Button>
            </div>
          )}
        </div>

        <div className="grid grid-cols-2 gap-3 rounded-xl border border-border bg-muted/30 p-3">
          <Select
            label={t('web.bind')}
            value={draft.bind ?? '127.0.0.1'}
            options={bindOptions}
            onChange={(value) => setDraft((prev) => ({ ...prev, bind: value }))}
            disabled={busyAll}
          />
          <label className="flex flex-col gap-1.5">
            <span className="text-xs font-medium text-muted-foreground">{t('web.domain')}</span>
            <Input
              value={draft.domain ?? 'test'}
              onChange={(event) => setDraft((prev) => ({ ...prev, domain: event.target.value }))}
              disabled={busyAll}
            />
          </label>
          <label className="flex flex-col gap-1.5">
            <span className="text-xs font-medium text-muted-foreground">{t('web.httpPort')}</span>
            <Input
              type="number"
              min={1}
              max={65535}
              value={draft.http_port ?? DEFAULT_HTTP_PORT}
              onChange={(event) => setDraft((prev) => ({ ...prev, http_port: Number(event.target.value) }))}
              disabled={busyAll}
            />
          </label>
          <label className="flex flex-col gap-1.5">
            <span className="text-xs font-medium text-muted-foreground">{t('web.httpsPort')}</span>
            <Input
              type="number"
              min={1}
              max={65535}
              value={draft.https_port ?? DEFAULT_HTTPS_PORT}
              onChange={(event) => setDraft((prev) => ({ ...prev, https_port: Number(event.target.value) }))}
              disabled={busyAll || !draft.https}
            />
          </label>
        </div>

        <div className="grid gap-3 rounded-xl border border-border bg-muted/30 p-3 sm:grid-cols-2">
          <Switch
            checked={draft.https ?? true}
            onChange={(checked) => setDraft((prev) => ({
              ...prev,
              https: checked,
              redirect_http: checked ? (prev.redirect_http ?? true) : false,
            }))}
            label={t('web.https.enable')}
            description={t('web.https.help')}
            disabled={busyAll}
          />
          <Switch
            checked={draft.redirect_http ?? true}
            onChange={(checked) => setDraft((prev) => ({ ...prev, redirect_http: checked }))}
            label={t('web.redirect')}
            description={t('web.redirect.help')}
            disabled={busyAll || !draft.https}
          />
        </div>
          </div>

          <div className="flex flex-col gap-3">
            <SectionHeading icon={<Code2 className="size-3.5" aria-hidden="true" />}>
              {t('lair.config.section.environ')}
            </SectionHeading>
        <div className="grid gap-3 rounded-xl border border-border bg-muted/30 p-3 sm:grid-cols-2">
          <Select
            label={t('lair.config.php')}
            value={defaults.php ?? ''}
            options={phpOptions.map((value) => ({ value, label: value }))}
            onChange={(value) => { void state.act(t('lair.config.php'), (c) => c.usePHP(value)); }}
            disabled={busyAll || phpOptions.length === 0}
          />
          <Select
            label={t('lair.config.node')}
            value={defaults.node ?? ''}
            options={nodeOptions.map((value) => ({ value, label: value }))}
            onChange={(value) => { void state.act(t('lair.config.node'), (c) => c.useNode(value)); }}
            disabled={busyAll || nodeOptions.length === 0}
          />
        </div>

        <div className="rounded-xl border border-border bg-muted/30 p-3">
          <div className="flex flex-wrap items-center justify-between gap-2">
            <div>
              <p className="text-sm font-medium">{t('lair.config.dirs')}</p>
              <p className="text-xs text-muted-foreground">{t('general.folders.importHelp')}</p>
            </div>
            <Button
              variant="outline"
              size="sm"
              disabled={busyAll}
              onClick={() => {
                void state.act(t('general.folders.import'), async (c) => {
                  const result = await c.projectImportValet();
                  const next = await c.projectDirs();
                  if (aliveRef.current) setDirs(next.dirs ?? []);
                  return result;
                });
              }}
            >
              {t('general.folders.import')}
            </Button>
          </div>
          {dirs.length === 0 ? (
            <p className="mt-2 text-xs text-muted-foreground">{t('general.folders.empty')}</p>
          ) : (
            <div className="mt-2">
              <CodeBlock text={dirs.join('\n')} />
            </div>
          )}
        </div>

            <SectionHeading icon={<Network className="size-3.5" aria-hidden="true" />}>
              {t('lair.config.section.diagnostics')}
            </SectionHeading>
        <div className="grid gap-3 rounded-xl border border-border bg-muted/30 p-3 sm:grid-cols-2">
          <div>
            <p className="text-sm font-medium">{t('lair.config.dns')}</p>
            <div className="mt-2 flex flex-wrap gap-2">
              <Badge tone={dns?.ok ? 'green' : 'orange'} dot>
                {dns?.ok ? t('lair.config.dns.ok') : t('lair.config.dns.missing')}
              </Badge>
              <span className="text-xs text-muted-foreground">{dns?.host ?? 'lair.test'}</span>
            </div>
            {(dns?.resolved?.length ?? 0) > 0 && (
              <p className="mt-1 text-xs text-muted-foreground">{dns?.resolved?.join(', ')}</p>
            )}
          </div>
          <div>
            <p className="text-sm font-medium">{t('lair.config.ca')}</p>
            <div className="mt-2 flex flex-wrap gap-2">
              <Badge tone={ca?.exists ? 'green' : 'orange'} dot>
                {ca?.exists ? t('lair.config.ca.ok') : t('lair.config.ca.missing')}
              </Badge>
              <Badge tone={ca?.system_trusted ? 'green' : 'orange'} dot>{t('lair.config.ca.system')}</Badge>
              <Badge tone={ca?.browser_trusted ? 'green' : 'orange'} dot>{t('lair.config.ca.browser')}</Badge>
            </div>
            {(ca?.site_certs ?? 0) > 0 && (
              <p className="mt-1 text-xs text-muted-foreground">{t('lair.config.ca.certs', { count: ca?.site_certs })}</p>
            )}
          </div>
        </div>

        <div className="rounded-xl border border-border p-3">
          <p className="text-sm font-medium">{t('lair.config.repair')}</p>
          <p className="mb-2 text-xs text-muted-foreground">{t('lair.config.repairHelp')}</p>
          <CodeBlock text={repair} />
        </div>
          </div>
        </div>

        <p className="text-xs text-muted-foreground">{t('web.applyHelp')}</p>
        {error && <p role="alert" className="text-xs text-red-600 dark:text-red-400">{error}</p>}
      </div>
    </Dialog>
  );
}
