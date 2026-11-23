import { useEffect, useRef, useState } from 'react';
import { Activity, Braces, Database, Download, FolderOpen, FolderTree, Globe2, HardDrive, Languages, ListTodo, Mail, Network, PlugZap, Plus, Trash2 } from 'lucide-react';
import type { DaemonClient, LairStatus, ProjectResult } from '../api';
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
  Input,
  Skeleton,
  StatCard,
  StatusDot,
} from '../components';
import { useI18n, useT } from '../i18n';
import type { PaneProps } from './types';

function counts(s: LairStatus | null) {
  const services = s?.services ?? {};
  const ids = Object.keys(services);
  return { running: ids.filter((id) => services[id]?.running).length, total: ids.length };
}

/** General pane: the daemon, domain, ports, paths and language in one place. */
export function OverviewPane({ state, onNavigate }: PaneProps) {
  const t = useT();
  const { locale } = useI18n();
  const s = state.status;
  const { running, total } = counts(s);
  const defaults = s?.defaults ?? {};
  const jobs = s?.jobs ?? [];

  const [folders, setFolders] = useState<string[]>([]);
  const [foldersLoaded, setFoldersLoaded] = useState(false);
  const [notice, setNotice] = useState('');
  const [actionError, setActionError] = useState('');
  const [busyFolders, setBusyFolders] = useState(false);
  const [addOpen, setAddOpen] = useState(false);
  const [newDir, setNewDir] = useState('');
  const [removeDir, setRemoveDir] = useState('');
  const [migrateOpen, setMigrateOpen] = useState(false);
  const aliveRef = useRef(true);

  useEffect(() => {
    aliveRef.current = true;
    return () => {
      aliveRef.current = false;
    };
  }, []);

  useEffect(() => {
    if (!state.client) return;
    let alive = true;
    setFoldersLoaded(false);
    void state.client.projectDirs().then((result) => {
      if (!alive) return;
      setFolders(result.dirs ?? []);
      setFoldersLoaded(true);
    }).catch(() => {
      // Older daemons have no dirs op: fall back to the folders in the status.
      if (alive) setFoldersLoaded(true);
    });
    return () => {
      alive = false;
    };
  }, [state.client]);

  const runFolder = async (
    fn: (client: DaemonClient) => Promise<ProjectResult>,
    onDone?: (result: ProjectResult) => void,
  ) => {
    if (!state.client) return;
    setBusyFolders(true);
    setActionError('');
    setNotice('');
    try {
      const result = await fn(state.client);
      if (!aliveRef.current) return;
      if (result.dirs) setFolders(result.dirs);
      if (onDone) onDone(result);
      else setNotice(result.output ?? '');
      await state.refresh();
    } catch (cause) {
      if (aliveRef.current) {
        setActionError(t('general.folders.error', {
          error: cause instanceof Error ? cause.message : String(cause),
        }));
      }
    } finally {
      if (aliveRef.current) setBusyFolders(false);
    }
  };

  if (!s) {
    return (
      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
        {Array.from({ length: 6 }).map((_, index) => (
          <Card key={index} className="p-5">
            <Skeleton className="mb-4 h-3 w-24" />
            <Skeleton className="h-8 w-16" />
          </Card>
        ))}
      </div>
    );
  }

  const web = s.web;
  const siteFolders = (s.sites ?? []).map((site) => site.path).filter((path): path is string => Boolean(path));
  const shownFolders = foldersLoaded
    ? folders
    : [...new Set([defaults.projects_dir, ...siteFolders].filter((dir): dir is string => Boolean(dir)))];
  const clean = (dir: string) => dir.replace(/\/+$/, '');
  const legacyDir = shownFolders.find((dir) => /(^|\/)lair$/.test(clean(dir)));
  const migrateTo = shownFolders.find((dir) => /(^|\/)Lair$/.test(clean(dir)))
    ?? (legacyDir ? clean(legacyDir).replace(/\/lair$/, '/Lair') : '');

  return (
    <div className="flex flex-col gap-5">
      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
        <StatCard label={t('dash.services')} value={`${running} / ${total}`} icon={<Activity />} />
        <StatCard label={t('dash.sites')} value={String((s.sites ?? []).length)} icon={<FolderOpen />} />
        <StatCard label={t('dash.jobs')} value={String(jobs.length)} icon={<ListTodo />} />
        <StatCard label={t('dash.phpVersion')} value={defaults.php || t('dash.none')} icon={<Braces />} />
        <StatCard label={t('dash.nodeVersion')} value={defaults.node || t('dash.none')} icon={<HardDrive />} />
        <StatCard label={t('dash.database')} value={defaults.database || t('dash.none')} icon={<Database />} />
      </div>

      <div className="grid gap-4 lg:grid-cols-2">
        <Card>
          <CardHeader>
            <CardTitle>{t('dash.home')}</CardTitle>
            <CardDescription>{t('pane.overview.subtitle')}</CardDescription>
          </CardHeader>
          <CardContent>
            <CodeBlock text={s.home || t('dash.none')} />
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>{t('pane.services')}</CardTitle>
            <CardDescription>{t('pane.services.subtitle')}</CardDescription>
          </CardHeader>
          <CardContent className="flex flex-wrap gap-2">
            {Object.entries(s.services ?? {}).map(([id, service]) => (
              <Badge key={id} tone={service.running ? 'green' : 'neutral'} dot>
                {id}
              </Badge>
            ))}
          </CardContent>
        </Card>
      </div>

      <Card>
        <CardHeader className="flex-row items-start justify-between space-y-0">
          <div>
            <CardTitle>{t('general.title')}</CardTitle>
            <CardDescription>{t('general.subtitle')}</CardDescription>
          </div>
          <Button variant="outline" size="sm" disabled={!onNavigate} onClick={() => onNavigate?.('mail')}>
            <Mail className="size-3.5" aria-hidden="true" />
            {t('mail.open')}
          </Button>
        </CardHeader>
        <CardContent className="flex flex-col gap-4">
          <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
            <div className="flex items-center justify-between gap-2 rounded-lg border border-border bg-muted/30 px-3 py-2">
              <span className="flex items-center gap-1.5 text-xs text-muted-foreground">
                <PlugZap className="size-3.5" aria-hidden="true" />
                {t('general.daemon')}
              </span>
              <span className="flex items-center gap-1.5 text-xs font-medium">
                <StatusDot on={Boolean(state.status && !state.error)} />
                {state.status && !state.error ? t('sidebar.connected') : t('dash.waiting')}
              </span>
            </div>
            <div className="flex items-center justify-between gap-2 rounded-lg border border-border bg-muted/30 px-3 py-2">
              <span className="flex items-center gap-1.5 text-xs text-muted-foreground">
                <Globe2 className="size-3.5" aria-hidden="true" />
                {t('general.domain')}
              </span>
              <span className="truncate text-xs font-medium">*.{web?.domain || 'test'}</span>
            </div>
            <div className="flex items-center justify-between gap-2 rounded-lg border border-border bg-muted/30 px-3 py-2">
              <span className="flex items-center gap-1.5 text-xs text-muted-foreground">
                <Network className="size-3.5" aria-hidden="true" />
                {t('general.ports')}
              </span>
              <span className="text-xs font-medium">
                {web?.http_port ?? 80} / {web?.https_port ?? 443}
              </span>
            </div>
            <div className="flex items-center justify-between gap-2 rounded-lg border border-border bg-muted/30 px-3 py-2">
              <span className="flex items-center gap-1.5 text-xs text-muted-foreground">
                <Languages className="size-3.5" aria-hidden="true" />
                {t('general.language')}
              </span>
              <span className="text-xs font-medium">{locale.toUpperCase()}</span>
            </div>
            <div className="flex items-center justify-between gap-2 rounded-lg border border-border bg-muted/30 px-3 py-2">
              <span className="flex items-center gap-1.5 text-xs text-muted-foreground">
                <Mail className="size-3.5" aria-hidden="true" />
                {t('mail.count')}
              </span>
              <span className="text-xs font-medium">{s.mail_count ?? '—'}</span>
            </div>
            <div className="flex items-center justify-between gap-2 rounded-lg border border-border bg-muted/30 px-3 py-2">
              <span className="flex items-center gap-1.5 text-xs text-muted-foreground">
                <Activity className="size-3.5" aria-hidden="true" />
                {t('general.debugger')}
              </span>
              <span className="text-xs font-medium">{s.debug_running ? t('common.running') : t('common.stopped')}</span>
            </div>
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader className="flex-row items-start justify-between space-y-0">
          <div>
            <CardTitle className="flex items-center gap-2">
              <FolderTree className="size-4" aria-hidden="true" />
              {t('general.projectFolders')}
            </CardTitle>
            <CardDescription>{t('general.folders.subtitle')}</CardDescription>
          </div>
          <div className="flex flex-wrap justify-end gap-2">
            <Button
              variant="outline"
              size="sm"
              disabled={!state.client || busyFolders || !!state.busy}
              onClick={() => setAddOpen(true)}
            >
              <Plus className="size-3.5" aria-hidden="true" />
              {t('general.folders.add')}
            </Button>
            <Button
              variant="outline"
              size="sm"
              disabled={!state.client || busyFolders || !!state.busy}
              onClick={() => void runFolder((client) => client.projectImportValet(), (result) => {
                setNotice(t('general.folders.imported', { output: result.output ?? '' }));
              })}
            >
              <Download className="size-3.5" aria-hidden="true" />
              {t('general.folders.import')}
            </Button>
            {legacyDir && (
              <Button
                variant="outline"
                size="sm"
                disabled={!state.client || busyFolders || !!state.busy || !migrateTo}
                onClick={() => setMigrateOpen(true)}
              >
                {t('general.folders.migrate')}
              </Button>
            )}
          </div>
        </CardHeader>
        <CardContent className="flex flex-col gap-3">
          {shownFolders.length === 0 ? (
            <p className="text-xs text-muted-foreground">{t('general.folders.empty')}</p>
          ) : (
            <ul className="flex flex-col gap-2">
              {shownFolders.map((dir) => (
                <li key={dir} className="flex items-center justify-between gap-2 rounded-lg border border-border bg-muted/30 px-3 py-2">
                  <span className="flex min-w-0 items-center gap-2 text-xs">
                    <FolderTree className="size-3.5 shrink-0 text-muted-foreground" aria-hidden="true" />
                    <span className="truncate font-medium">{dir}</span>
                  </span>
                  <Button
                    variant="ghost"
                    size="sm"
                    disabled={!state.client || busyFolders || !!state.busy}
                    aria-label={`${t('general.folders.remove')} ${dir}`}
                    onClick={() => setRemoveDir(dir)}
                  >
                    <Trash2 className="size-3.5" aria-hidden="true" />
                  </Button>
                </li>
              ))}
            </ul>
          )}
          {notice && <p role="status" className="text-xs text-muted-foreground">{notice}</p>}
          {actionError && <p role="alert" className="text-xs text-red-600 dark:text-red-400">{actionError}</p>}
        </CardContent>
      </Card>

      <Dialog
        open={addOpen}
        onClose={() => { setAddOpen(false); setNewDir(''); }}
        title={t('general.folders.addTitle')}
        description={t('general.folders.addHelp')}
        footer={
          <>
            <Button variant="ghost" onClick={() => { setAddOpen(false); setNewDir(''); }}>
              {t('common.cancel')}
            </Button>
            <Button
              disabled={!newDir.trim()}
              onClick={() => {
                const dir = newDir.trim();
                setAddOpen(false);
                setNewDir('');
                void runFolder((client) => client.projectDirsAdd(dir));
              }}
            >
              {t('general.folders.save')}
            </Button>
          </>
        }
      >
        <label className="flex flex-col gap-1.5">
          <span className="text-xs font-medium text-muted-foreground">{t('general.folders.path')}</span>
          <Input
            autoFocus
            value={newDir}
            placeholder="~/Lair"
            onChange={(event) => setNewDir(event.target.value)}
          />
        </label>
      </Dialog>

      <Dialog
        open={Boolean(removeDir)}
        onClose={() => setRemoveDir('')}
        title={t('general.folders.removeTitle')}
        description={t('general.folders.removeMessage', { dir: removeDir })}
        footer={
          <>
            <Button variant="ghost" onClick={() => setRemoveDir('')}>{t('common.cancel')}</Button>
            <Button
              variant="danger"
              disabled={busyFolders}
              onClick={() => {
                const dir = removeDir;
                setRemoveDir('');
                void runFolder((client) => client.projectDirsRemove(dir));
              }}
            >
              {t('general.folders.remove')}
            </Button>
          </>
        }
      />

      <Dialog
        open={migrateOpen}
        onClose={() => setMigrateOpen(false)}
        title={t('general.folders.migrateTitle')}
        description={t('general.folders.migrateMessage', { from: legacyDir ?? '', to: migrateTo })}
        footer={
          <>
            <Button variant="ghost" onClick={() => setMigrateOpen(false)}>{t('common.cancel')}</Button>
            <Button
              disabled={busyFolders || !legacyDir || !migrateTo}
              onClick={() => {
                const from = legacyDir ?? '';
                const to = migrateTo;
                setMigrateOpen(false);
                void runFolder((client) => client.projectMigrateDirs(from, to));
              }}
            >
              {t('general.folders.migrate')}
            </Button>
          </>
        }
      />
    </div>
  );
}
