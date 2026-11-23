import { useState } from 'react';
import { Database, Plus, Trash2 } from 'lucide-react';
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
  Input,
  ServiceInstallForm,
} from '../components';
import type { CatalogLineOption } from '../components';
import { useT } from '../i18n';
import { ServiceToggle } from './ServiceToggle';
import type { PaneProps } from './types';
import { useVersions } from './useVersions';

const DB_IDS = ['mariadb', 'mysql', 'postgres'] as const;
const DEFAULT_PORTS: Record<string, number> = { mariadb: 3306, mysql: 3307, postgres: 5432 };
const LABELS: Record<string, string> = { mariadb: 'MariaDB', mysql: 'MySQL', postgres: 'PostgreSQL' };

export function DatabasesPane({ state }: PaneProps) {
  const t = useT();
  const services = state.status?.services ?? {};
  const defaults = state.status?.defaults ?? {};
  const catalog = useVersions(state.client, [...DB_IDS]);
  const [selected, setSelected] = useState('');
  const [port, setPort] = useState(0);
  const [createFor, setCreateFor] = useState<string | null>(null);
  const [dbName, setDbName] = useState('');
  const [uninstall, setUninstall] = useState<{ id: string; version: string } | null>(null);

  const lines: CatalogLineOption[] = DB_IDS.flatMap((id) =>
    (catalog[id] ?? []).map((version) => ({
      id: `${id}:${version}`,
      label: `${LABELS[id]} ${version}`,
      latest: version,
    })),
  );
  const current = selected || lines[0]?.id || '';
  const engine = current.split(':')[0] || 'mariadb';
  const effectivePort = port || services[engine]?.port || DEFAULT_PORTS[engine] || 3306;

  const selectLine = (id: string) => {
    setSelected(id);
    const nextEngine = id.split(':')[0];
    setPort(services[nextEngine]?.port || DEFAULT_PORTS[nextEngine] || 3306);
  };

  return (
    <div className="flex flex-col gap-5">
      <Card>
        <CardHeader>
          <CardTitle>{t('services.install.title')}</CardTitle>
          <CardDescription>{t('pane.databases.subtitle')}</CardDescription>
        </CardHeader>
        <CardContent>
          {lines.length > 0 ? (
            <ServiceInstallForm
              name={LABELS[engine] ?? engine}
              lines={lines}
              selectedLine={current}
              onSelectLine={selectLine}
              port={effectivePort}
              onPortChange={setPort}
              defaultPort={services[engine]?.port || DEFAULT_PORTS[engine] || 3306}
              allowPort={engine !== 'postgres'}
              busy={!!state.busy}
              onInstall={(line, chosenPort) => {
                const [id, version] = line.split(':');
                if (!id || !version) return;
                void state.act(t('dash.installing', { name: `${LABELS[id] ?? id} ${version}` }), (c) =>
                  c.installService(id, version, chosenPort),
                );
              }}
            />
          ) : (
            <EmptyState title={t('dash.waiting')} />
          )}
        </CardContent>
      </Card>

      {defaults.database !== 'sqlite' && (
        <Card>
          <CardHeader>
            <CardTitle className="text-base">SQLite</CardTitle>
            <CardDescription>{t('pane.databases.subtitle')}</CardDescription>
          </CardHeader>
          <CardContent>
            <Button variant="outline" size="sm" disabled={!!state.busy}
              onClick={() => void state.act(t('dash.switching', { name: 'SQLite' }), (c) => c.setDefaultDB('sqlite'))}>
              {t('common.makeDefault')}
            </Button>
          </CardContent>
        </Card>
      )}

      {DB_IDS.filter((id) => services[id]).length === 0 ? (
        <EmptyState icon={<Database />} title={t('dash.noServices')} />
      ) : (
        <div className="grid gap-4 lg:grid-cols-2">
          {DB_IDS.filter((id) => services[id]).map((id) => {
            const svc = services[id];
            const isDefault = defaults.database === id;
            const versions = installedVersions(svc);
            return (
              <Card key={id}>
                <CardHeader>
                  <div className="flex items-start justify-between gap-3">
                    <div>
                      <CardTitle className="text-base">{LABELS[id] ?? id}</CardTitle>
                      <CardDescription>
                        {t('dash.version')}: {activeVersion(svc) || '—'} · {t('dash.port')}: {svc.port ?? '—'}
                      </CardDescription>
                    </div>
                    {isDefault && <Badge tone="green" dot>{t('common.default')}</Badge>}
                  </div>
                  <ServiceToggle id={id} service={svc} state={state} />
                </CardHeader>
                <CardContent className="flex flex-col gap-3">
                  {versions.length > 1 && (
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
                  <div className="flex flex-wrap gap-2">
                    {!isDefault && (
                      <Button variant="outline" size="sm" disabled={!!state.busy} onClick={() => void state.act(t('dash.switching', { name: id }), (c) => c.setDefaultDB(id))}>
                        {t('common.makeDefault')}
                      </Button>
                    )}
                    <Button variant="subtle" size="sm" onClick={() => { setCreateFor(id); setDbName(''); }}>
                      <Plus className="size-3.5" aria-hidden="true" />
                      {t('dash.createDB')}
                    </Button>
                    {versions.length > 0 && (
                      <ActionsMenu
                        label={t('common.actions', { name: id })}
                        actions={versions.map((version) => ({
                          label: `${t('common.uninstall')} ${version}`,
                          destructive: true,
                          disabled: !!state.busy,
                          onSelect: () => setUninstall({ id, version }),
                        }))}
                      />
                    )}
                  </div>
                </CardContent>
              </Card>
            );
          })}
        </div>
      )}

      <Dialog
        open={!!createFor}
        onClose={() => setCreateFor(null)}
        title={t('dash.createDBFor', { name: createFor ?? '' })}
        size="sm"
        footer={
          <>
            <Button variant="ghost" onClick={() => setCreateFor(null)}>{t('common.cancel')}</Button>
            <Button disabled={!dbName.trim()} onClick={() => {
              if (!createFor || !dbName.trim()) return;
              void state.act(t('dash.creatingDB', { name: dbName }), (c) => c.createDB(createFor, dbName.trim()));
              setCreateFor(null);
            }}>{t('common.save')}</Button>
          </>
        }
      >
        <label className="flex flex-col gap-1.5">
          <span className="text-xs font-medium text-muted-foreground">{t('dash.dbName')}</span>
          <Input autoFocus value={dbName} placeholder="acme_dev" onChange={(event) => setDbName(event.target.value)} />
        </label>
      </Dialog>

      <Dialog
        open={!!uninstall}
        onClose={() => setUninstall(null)}
        title={t('dash.confirmUninstall', { name: uninstall ? `${uninstall.id} ${uninstall.version}` : '' })}
        description={t('services.uninstall.keepData')}
        size="sm"
        footer={
          <>
            <Button variant="ghost" onClick={() => setUninstall(null)}>{t('common.cancel')}</Button>
            <Button variant="danger" onClick={() => {
              if (!uninstall) return;
              void state.act(t('dash.uninstalling', { name: uninstall.version }), (c) => c.uninstallService(uninstall.id, uninstall.version));
              setUninstall(null);
            }}>
              <Trash2 className="size-4" aria-hidden="true" />
              {t('common.uninstall')}
            </Button>
          </>
        }
      />
    </div>
  );
}
