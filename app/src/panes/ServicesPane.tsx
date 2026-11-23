import { useState } from 'react';
import { Download, FileText, Info, RotateCw, Server } from 'lucide-react';
import { activeVersion, installedVersions } from '../api';
import {
  ActionsMenu,
  Badge,
  Button,
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
  Dialog,
  EmptyState,
  ServiceInstallForm,
  Tabs,
  TabsList,
  TabsPanel,
  TabsPanels,
  TabsTrigger,
} from '../components';
import type { CatalogLineOption } from '../components';
import { useT } from '../i18n';
import { ServiceToggle } from './ServiceToggle';
import type { PaneProps } from './types';
import { useVersions } from './useVersions';

// Services that come from the signed catalog. Built-in ones (web, dns) are
// managed by Lair itself and never show install/build state.
const CATALOG_SERVICES = new Set([
  'mariadb', 'mysql', 'postgres', 'mongodb', 'redis', 'valkey',
  'meilisearch', 'typesense', 'rustfs',
]);

// SQLite is a file-based project database, not a supervised process. It gets
// an informational card so it is discoverable from the Services tab.
const SQLITE_ID = 'sqlite3';

const CATEGORY: Record<string, string> = {
  web: 'services.category.core',
  dns: 'services.category.core',
  mariadb: 'services.category.database',
  mysql: 'services.category.database',
  postgres: 'services.category.database',
  mongodb: 'services.category.database',
  redis: 'services.category.cacheQueue',
  valkey: 'services.category.cacheQueue',
  meilisearch: 'services.category.search',
  typesense: 'services.category.search',
  rustfs: 'services.category.storage',
  [SQLITE_ID]: 'services.category.database',
};

export function ServicesPane({ state }: PaneProps) {
  const t = useT();
  const services = state.status?.services ?? {};
  const ids = Object.keys(services).sort();
  const catalogIds = ids.includes(SQLITE_ID) ? ids : [...ids, SQLITE_ID];
  const catalog = useVersions(state.client, catalogIds);
  const [installFor, setInstallFor] = useState<string | null>(null);
  const [installLine, setInstallLine] = useState('');
  const [installPort, setInstallPort] = useState(0);
  const [log, setLog] = useState<{ name: string; text: string } | null>(null);

  const linesFor = (id?: string): CatalogLineOption[] =>
    catalogIds
      .filter((candidate) => !id || candidate === id)
      .flatMap((candidate) =>
        (catalog[candidate] ?? []).map((version) => ({
          id: `${candidate}:${version}`,
          label: `${candidate} ${version}`,
          latest: version,
        })),
      );

  const modalId = installFor === 'all' || !installFor ? undefined : installFor;
  const modalLines = linesFor(modalId);
  const modalDefaultPort = modalId ? (services[modalId]?.port ?? 0) : 0;

  const openInstall = (id?: string) => {
    const lines = linesFor(id);
    setInstallLine(lines[0]?.id ?? '');
    setInstallPort(id ? (services[id]?.port ?? 0) : 0);
    setInstallFor(id ?? 'all');
  };

  const isInstalled = (id: string) => {
    const svc = services[id];
    if (!svc) return false;
    return svc.state === 'installed' || installedVersions(svc).length > 0 || Boolean(activeVersion(svc));
  };

  // Unavailable services stay visible with an explanation instead of silently
  // disappearing: the daemon marks them when the signed catalog has no build
  // for the current platform. Installed services always remain visible.
  const isUnavailable = (id: string) => services[id]?.available === false;
  const visible = [...ids, SQLITE_ID];

  const showLog = (id: string) => {
    void state.act(t('dash.readingLog', { name: id }), async (c) => {
      const text = await c.log(id);
      setLog({ name: id, text });
    });
  };

  const renderCards = (list: string[]) =>
    list.length === 0 ? (
      <EmptyState icon={<Server />} title={t('services.empty')} />
    ) : (
      <div className="grid gap-4 lg:grid-cols-2">
        {list.map((id) => {
          if (id === SQLITE_ID) {
            const svc = services[SQLITE_ID];
            const cliInstalled = svc ? installedVersions(svc).length > 0 || Boolean(activeVersion(svc)) : false;
            const sqliteLines = linesFor(SQLITE_ID);
            return (
              <Card key={id} className={cliInstalled ? undefined : 'border-dashed'}>
                <CardHeader>
                  <div className="flex items-start justify-between gap-3">
                    <div>
                      <CardTitle className="text-base">SQLite</CardTitle>
                      <CardDescription>{t(CATEGORY[SQLITE_ID]!)}</CardDescription>
                    </div>
                    <div className="flex flex-wrap justify-end gap-1.5">
                      <Badge tone="blue">{t('services.sqlite.ready')}</Badge>
                      {cliInstalled && (
                        <Badge tone="green">{t('services.sqlite.cliInstalled')}</Badge>
                      )}
                    </div>
                  </div>
                  <p className="text-xs text-muted-foreground">{t('services.sqlite.hint')}</p>
                </CardHeader>
                {sqliteLines.length > 0 && (
                  <CardContent>
                    <Button
                      variant={cliInstalled ? 'outline' : 'primary'}
                      size="sm"
                      disabled={!!state.busy}
                      aria-label={`${t('services.sqlite.installCli')} sqlite3`}
                      onClick={() => openInstall(SQLITE_ID)}
                    >
                      <Download className="size-3.5" aria-hidden="true" />
                      {t('services.sqlite.installCli')}
                    </Button>
                  </CardContent>
                )}
              </Card>
            );
          }
          const svc = services[id];
          const unavailable = isUnavailable(id);
          const installed = isInstalled(id);
          const versions = installedVersions(svc);
          const availableLines = linesFor(id);
          const meta = [
            `${t('dash.version')}: ${activeVersion(svc) || '—'}`,
            `${t('dash.port')}: ${svc.port ?? '—'}`,
            svc.pid ? `PID: ${svc.pid}` : '',
          ]
            .filter(Boolean)
            .join(' · ');
          return (
            <Card key={id} className={installed ? undefined : 'border-dashed'}>
              <CardHeader>
                <div className="flex items-start justify-between gap-3">
                  <div>
                    <CardTitle className="text-base">{id}</CardTitle>
                    <CardDescription>{t(CATEGORY[id] ?? 'services.category.core')}</CardDescription>
                  </div>
                  <div className="flex flex-wrap justify-end gap-1.5">
                    <Badge tone={installed ? 'blue' : 'neutral'}>
                      {installed
                        ? t('services.card.installed')
                        : unavailable
                          ? t('services.notAvailableOn', { platform: svc.platform ?? '—' })
                          : t('services.card.notInstalled')}
                    </Badge>
                    {installed && (
                      <Badge tone={svc.running ? 'green' : 'neutral'} dot>
                        {svc.running ? t('common.active') : t('common.stopped')}
                      </Badge>
                    )}
                  </div>
                </div>
                {installed ? (
                  <p className="text-xs text-muted-foreground">{meta}</p>
                ) : unavailable ? (
                  <p className="text-xs text-muted-foreground">
                    {svc.unavailable_reason === 'unsupported-platform'
                      ? t('services.card.unsupportedHint')
                      : t('services.card.noBuildHint')}
                  </p>
                ) : (
                  <p className="text-xs text-muted-foreground">{t('services.card.installHint', { name: id })}</p>
                )}
                {svc.last_error && <p className="text-xs text-destructive">{String(svc.last_error)}</p>}
                {installed && <ServiceToggle id={id} service={svc} state={state} />}
              </CardHeader>
              <CardContent className="flex flex-col gap-3">
                <div className="flex flex-wrap items-center gap-2">
                  {availableLines.length > 0 ? (
                    <Button
                      variant={installed ? 'outline' : 'primary'}
                      size="sm"
                      disabled={!!state.busy}
                      aria-label={`${installed ? t('services.install.another') : t('common.install')} ${id}`}
                      onClick={() => openInstall(id)}
                    >
                      <Download className="size-3.5" aria-hidden="true" />
                      {installed ? t('services.install.another') : t('common.install')}
                    </Button>
                  ) : CATALOG_SERVICES.has(id) ? (
                    <span
                      title={t('services.card.noBuildHint')}
                      className="inline-flex items-center gap-1.5 rounded-md border border-dashed border-border px-2.5 py-1.5 text-xs text-muted-foreground"
                    >
                      <Info className="size-3.5" aria-hidden="true" />
                      {t('services.card.noBuild')}
                    </span>
                  ) : null}
                  {installed && (
                    <Button variant="ghost" size="sm" onClick={() => showLog(id)}>
                      <FileText className="size-3.5" aria-hidden="true" />
                      {t('dash.viewLog')}
                    </Button>
                  )}
                  {installed && (
                    <ActionsMenu
                      label={t('common.actions', { name: id })}
                      actions={[
                        ...versions
                          .filter((version) => version !== activeVersion(svc))
                          .map((version) => ({
                            label: `${t('dash.useVersion')} ${version}`,
                            disabled: !!state.busy,
                            onSelect: () => void state.act(t('dash.switching', { name: version }), (c) => c.useService(id, version)),
                          })),
                        ...versions.map((version) => ({
                          label: `${t('common.uninstall')} ${version}`,
                          destructive: true,
                          disabled: !!state.busy,
                          onSelect: () => {
                            if (!window.confirm(t('dash.confirmUninstall', { name: `${id} ${version}` }))) return;
                            void state.act(t('dash.uninstalling', { name: version }), (c) => c.uninstallService(id, version));
                          },
                        })),
                      ]}
                    />
                  )}
                </div>
                {installed && versions.length > 1 && (
                  <ul className="flex flex-col gap-2">
                    {versions.map((version) => (
                      <li key={version} className="flex items-center justify-between gap-2 rounded-lg border border-border bg-muted/30 px-3 py-2">
                        <span className="truncate text-sm font-medium">{version}</span>
                        {version === activeVersion(svc) ? (
                          <Badge tone="green">{t('services.line.active')}</Badge>
                        ) : (
                          <Button variant="ghost" size="sm" disabled={!!state.busy} onClick={() => void state.act(t('dash.switching', { name: version }), (c) => c.useService(id, version))}>
                            {t('dash.useVersion')}
                          </Button>
                        )}
                      </li>
                    ))}
                  </ul>
                )}
              </CardContent>
            </Card>
          );
        })}
      </div>
    );

  const installed = visible.filter(isInstalled);
  const more = visible.filter((id) => id !== SQLITE_ID && !isInstalled(id));

  return (
    <div className="flex flex-col gap-5">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div className="flex flex-wrap gap-2">
          <Button variant="primary" size="sm" disabled={!!state.busy} onClick={() => openInstall()}>
            <Download className="size-3.5" aria-hidden="true" />
            {t('common.install')}
          </Button>
          <Button variant="outline" size="sm" disabled={!!state.busy} onClick={() => {
            if (!window.confirm(t('dash.confirmRestartAll'))) return;
            void state.act(t('dash.restartingAll'), (c) => c.restart());
          }}>
            <RotateCw className="size-3.5" aria-hidden="true" />
            {t('dash.restartAll')}
          </Button>
          <Button variant="ghost" size="sm" disabled={!!state.busy} onClick={() => void state.refresh()}>
            {t('dash.refresh')}
          </Button>
        </div>
        <p className="text-xs text-muted-foreground">
          {t('services.summary', { installed: installed.length, total: visible.length })}
        </p>
      </div>

      <Tabs defaultIndex={1}>
        <TabsList>
          <TabsTrigger>{t('services.filter.installed')}</TabsTrigger>
          <TabsTrigger>{t('services.filter.all')}</TabsTrigger>
          <TabsTrigger>{t('services.filter.more')}</TabsTrigger>
        </TabsList>
        <TabsPanels>
          <TabsPanel>{renderCards(installed)}</TabsPanel>
          <TabsPanel>{renderCards(visible)}</TabsPanel>
          <TabsPanel>{renderCards(more)}</TabsPanel>
        </TabsPanels>
      </Tabs>

      <Dialog
        open={!!installFor}
        onClose={() => setInstallFor(null)}
        title={t('services.install.title')}
        description={modalId ? t('services.card.installHint', { name: modalId }) : t('pane.services.subtitle')}
        size="md"
      >
        {modalLines.length > 0 ? (
          <ServiceInstallForm
            name={modalId ?? 'service'}
            lines={modalLines}
            selectedLine={installLine}
            onSelectLine={setInstallLine}
            port={installPort}
            onPortChange={setInstallPort}
            defaultPort={modalDefaultPort}
            allowPort={modalDefaultPort > 0}
            busy={!!state.busy}
            onInstall={(line, port) => {
              const [id, version] = line.split(':');
              if (!id || !version) return;
              void state.act(t('dash.installing', { name: `${id} ${version}` }), (c) =>
                c.installService(id, version, port || undefined));
              setInstallFor(null);
            }}
          />
        ) : (
          <EmptyState title={t('dash.waiting')} />
        )}
      </Dialog>

      <Dialog open={!!log} onClose={() => setLog(null)} title={log ? t('dash.logOf', { name: log.name }) : ''} size="lg">
        <pre className="max-h-[55vh] overflow-auto rounded-lg bg-muted/40 p-4 text-xs leading-relaxed">{log?.text || t('dash.noLog')}</pre>
      </Dialog>
    </div>
  );
}
