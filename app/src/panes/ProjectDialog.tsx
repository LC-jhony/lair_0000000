import { useEffect, useRef, useState } from 'react';
import { Button, Dialog, Input, Progress, Select, Switch } from '../components';
import { jobProgress, type DaemonClient, type JobStatus, type ProjectCreatePayload, type ProjectInfo } from '../api';
import { useT } from '../i18n';
import type { LairController } from '../useLair';

const NAME_RE = /^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$/;
const DB_RE = /^[A-Za-z_][A-Za-z0-9_]*$/;

interface Props {
  open: boolean;
  onClose: () => void;
  state: LairController;
}

type Form = {
  name: string;
  parent: string;
  secure: boolean;
  stack: ProjectCreatePayload['stack'];
  auth: boolean;
  testing: ProjectCreatePayload['testing'];
  boost: boolean;
  nodeManager: ProjectCreatePayload['node_manager'];
  git: boolean;
  database: ProjectCreatePayload['database'];
  dbName: string;
  dbUser: string;
  dbPassword: string;
  dbHost: string;
  dbPort: string;
  createDB: boolean;
  startService: boolean;
};

const emptyForm: Form = {
  name: '', parent: '', secure: true, stack: 'blade', auth: true,
  testing: 'pest', boost: false, nodeManager: 'npm', git: false,
  database: 'sqlite', dbName: '', dbUser: 'root', dbPassword: '',
  dbHost: '127.0.0.1', dbPort: '', createDB: true, startService: true,
};

/** Four-step wizard mirroring Valet Deck's New project, but backed by Lair's
 * single `project` daemon action and its existing job/log/cancel actions. */
export function ProjectDialog({ open, onClose, state }: Props) {
  const t = useT();
  const [info, setInfo] = useState<ProjectInfo>({});
  const [form, setForm] = useState<Form>(emptyForm);
  const [step, setStep] = useState(0);
  const [jobID, setJobID] = useState('');
  const [job, setJob] = useState<JobStatus>({});
  const [log, setLog] = useState('');
  const [site, setSite] = useState('');
  const [error, setError] = useState('');
  // Keeps the submitted credentials available for Retry after we clear the
  // password from the visible form.
  const lastPayloadRef = useRef<ProjectCreatePayload | null>(null);

  const set = <K extends keyof Form>(key: K, value: Form[K]) =>
    setForm((prev) => ({ ...prev, [key]: value }));

  useEffect(() => {
    if (!open || !state.client) return;
    let alive = true;
    void state.client.projectInfo().then((result) => {
      if (!alive) return;
      setInfo(result);
      // The daemon owns the default folder: use it unless the user already
      // typed a parent. Never default to the current directory or LAIR_HOME.
      const projectsDir = result.projects_dir?.trim();
      if (projectsDir) {
        setForm((prev) => (prev.parent.trim() ? prev : { ...prev, parent: projectsDir }));
      }
    }).catch(() => undefined);
    return () => { alive = false; };
  }, [open, state.client]);

  useEffect(() => {
    if (!jobID || !state.client) return;
    let alive = true;
    let timer = 0;
    const poll = async () => {
      try {
        const [status, text] = await Promise.all([
          state.client!.job(jobID),
          state.client!.log(`${jobID}-install.log`),
        ]);
        if (!alive) return;
        setJob(status);
        setLog(text);
        if (status.done || status.error || status.canceled) {
          window.clearInterval(timer);
          if (status.done && !status.error && !status.canceled) {
            setSite(jobID.replace(/^project-/, '') + '.test');
          }
        }
      } catch (cause) {
        if (alive) setError(cause instanceof Error ? cause.message : String(cause));
      }
    };
    void poll();
    timer = window.setInterval(() => void poll(), 500);
    return () => { alive = false; window.clearInterval(timer); };
  }, [jobID, state.client]);

  const close = () => {
    setForm(emptyForm);
    setStep(0);
    setJobID('');
    setJob({});
    setLog('');
    setSite('');
    setError('');
    lastPayloadRef.current = null;
    onClose();
  };

  const validate = (): string => {
    if (!NAME_RE.test(form.name)) return t('project.invalidName');
    // An empty parent is valid: the daemon resolves it to the configured
    // sites folder (default ~/Lair), never to the current directory.
    if (form.database !== 'sqlite') {
      if (!DB_RE.test(form.dbName)) return t('project.invalidDatabase');
      if (!form.dbUser.trim() || !form.dbPassword.trim()) return t('project.invalidCredentials');
      if (form.dbPort.trim()) {
        const port = Number(form.dbPort);
        if (!Number.isInteger(port) || port < 1 || port > 65535) return t('project.invalidPort');
      }
    }
    return '';
  };

  const start = async (client: DaemonClient, payload: ProjectCreatePayload) => {
    try {
      const result = await client.projectCreate(payload);
      setJobID(result.id ?? '');
      setJob(result);
      if (result.site) setSite(result.site);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : String(cause));
    }
  };

  const submit = async (client: DaemonClient) => {
    const problem = validate();
    if (problem) {
      setError(problem);
      return;
    }
    setError('');
    const payload: ProjectCreatePayload = {
      name: form.name.trim().toLowerCase(),
      parent: form.parent.trim(),
      secure: form.secure,
      stack: form.stack,
      auth: form.auth,
      testing: form.testing,
      boost: form.boost,
      node_manager: form.nodeManager,
      git: form.git,
      database: form.database,
      db_name: form.dbName.trim() || `${form.name.replace(/-/g, '_')}_dev`,
      db_user: form.dbUser.trim(),
      db_password: form.dbPassword,
      db_host: form.dbHost.trim() || '127.0.0.1',
      db_port: Number(form.dbPort) || undefined,
      create_db: form.createDB,
      start_service: form.startService,
    };
    lastPayloadRef.current = payload;
    set('dbPassword', '');
    await start(client, payload);
  };

  const retry = async () => {
    const payload = lastPayloadRef.current;
    if (!payload || !state.client) return;
    setJobID('');
    setJob({});
    setLog('');
    setSite('');
    setError('');
    await start(state.client, payload);
  };

  const options = (values: readonly string[]) => values.map((value) => ({ value, label: value }));
  const failed = Boolean(job.error || job.canceled);
  const running = Boolean(jobID) && !job.done && !failed;
  const progress = jobProgress(job);
  const steps = [t('project.stepProject'), t('project.stepInterface'), t('project.stepTools'), t('project.stepDatabase')];
  const databases = info.databases?.length ? info.databases : ['sqlite', 'mysql', 'mariadb', 'pgsql', 'sqlsrv'];
  const nodeManagers = info.node_managers?.length
    ? [...info.node_managers, 'none'].filter((value, index, all) => all.indexOf(value) === index)
    : ['npm', 'pnpm', 'bun', 'yarn', 'none'];

  const footer = running ? (
    <>
      <Button variant="outline" onClick={() => void state.client?.cancelJob(jobID)}>{t('project.cancelJob')}</Button>
      <Button disabled>{t('project.creating')}</Button>
    </>
  ) : failed ? (
    <>
      <Button variant="outline" onClick={() => void retry()}>{t('common.retry')}</Button>
      <Button onClick={close}>{t('common.close')}</Button>
    </>
  ) : site ? (
    <Button onClick={() => { void state.refresh(); close(); }}>{t('common.close')}</Button>
  ) : (
    <>
      <Button variant="ghost" onClick={close}>{t('common.cancel')}</Button>
      {step > 0 && <Button variant="outline" onClick={() => setStep(step - 1)}>{t('project.back')}</Button>}
      <Button
        disabled={!state.client}
        onClick={() => {
          if (step < 3) {
            const problem = step === 0 ? validate() : '';
            setError(problem);
            if (problem) return;
            setStep(step + 1);
          } else {
            void submit(state.client!);
          }
        }}
      >
        {step < 3 ? t('project.next') : t('project.create')}
      </Button>
    </>
  );

  return (
    <Dialog
      open={open}
      onClose={close}
      title={t('project.newProject')}
      description={t('project.newHelp')}
      footer={footer}
      size="lg"
    >
      {error && <div role="alert" className="mb-4 rounded-lg bg-destructive/10 p-3 text-sm text-destructive">{error}</div>}

      {info.laravel_installed && info.laravel_version && (
        <p className="mb-4 text-xs text-muted-foreground">{info.laravel_version}</p>
      )}
      {info.hint && (
        <p className="mb-4 rounded-lg bg-muted p-3 text-xs text-muted-foreground">{info.hint}</p>
      )}

      {running || site || failed ? (
        <div className="flex flex-col gap-4">
          <Progress value={(progress ?? 0) * 100} label={failed ? t('project.failed') : t('project.creating')} />
          {failed ? (
            <div role="alert" className="rounded-lg bg-destructive/10 p-3 text-sm text-destructive">
              <p className="font-medium">{job.canceled ? t('project.canceled') : t('project.failed')}</p>
              {job.error && <p className="mt-1 text-xs">{job.error}</p>}
            </div>
          ) : (
            <p className="text-sm text-muted-foreground">{site ? `${t('project.success')} ${site}` : t('project.creating')}</p>
          )}
          <pre className="max-h-64 overflow-auto rounded-lg bg-muted p-3 text-xs leading-relaxed">{log || t('project.waitingLog')}</pre>
        </div>
      ) : (
        <>
          <div className="mb-5 flex gap-2">
            {steps.map((label, index) => (
              <button
                key={index}
                type="button"
                onClick={() => setStep(index)}
                className={`flex-1 rounded-lg px-2 py-1.5 text-xs font-medium ${index === step ? 'bg-primary text-primary-foreground' : 'bg-muted text-muted-foreground'}`}
              >
                {index + 1}. {label}
              </button>
            ))}
          </div>

          {step === 0 && (
            <div className="grid gap-4">
              <label className="flex flex-col gap-1.5">
                <span className="text-xs font-medium text-muted-foreground">{t('project.name')}</span>
                <Input autoFocus value={form.name} placeholder="mi-app" onChange={(e) => set('name', e.target.value)} />
              </label>
              <label className="flex flex-col gap-1.5">
                <span className="text-xs font-medium text-muted-foreground">{t('project.parent')}</span>
                <Input value={form.parent} placeholder={info.projects_dir || '~/Lair'} onChange={(e) => set('parent', e.target.value)} />
                <span className="text-xs text-muted-foreground">{t('project.parentHelp')}</span>
              </label>
              <Switch checked={form.secure} onChange={(v) => set('secure', v)} label={t('project.https')} description={t('project.httpsHelp')} />
            </div>
          )}

          {step === 1 && (
            <div className="grid gap-4">
              <Select value={form.stack} onChange={(v) => set('stack', v as Form['stack'])} options={options(['blade', 'react', 'vue', 'svelte', 'livewire'])} label={t('project.stack')} />
              <Switch checked={form.auth} onChange={(v) => set('auth', v)} label={t('project.auth')} description={t('project.authHelp')} />
            </div>
          )}

          {step === 2 && (
            <div className="grid gap-4">
              <Select value={form.testing} onChange={(v) => set('testing', v as Form['testing'])} options={options(['pest', 'phpunit'])} label={t('project.testing')} />
              <Select value={form.nodeManager} onChange={(v) => set('nodeManager', v as Form['nodeManager'])} options={options(nodeManagers)} label={t('project.nodeManager')} />
              <Switch checked={form.boost} onChange={(v) => set('boost', v)} label={t('project.boost')} />
              <Switch checked={form.git} onChange={(v) => set('git', v)} label={t('project.git')} />
            </div>
          )}

          {step === 3 && (
            <div className="grid gap-4">
              <Select value={form.database} onChange={(v) => set('database', v as Form['database'])} options={options(databases)} label={t('project.database')} />
              {form.database === 'sqlite' ? (
                <p className="rounded-lg bg-muted p-3 text-xs text-muted-foreground">{t('project.sqliteHelp')}</p>
              ) : (
                <>
                  {form.database === 'sqlsrv' && (
                    <p className="rounded-lg bg-muted p-3 text-xs text-muted-foreground">{t('project.sqlServerHelp')}</p>
                  )}
                  <div className="grid gap-4 sm:grid-cols-2">
                    <label className="flex flex-col gap-1.5">
                      <span className="text-xs font-medium text-muted-foreground">{t('project.dbName')}</span>
                      <Input value={form.dbName} placeholder="mi_app_dev" onChange={(e) => set('dbName', e.target.value)} />
                    </label>
                    <label className="flex flex-col gap-1.5">
                      <span className="text-xs font-medium text-muted-foreground">{t('project.dbUser')}</span>
                      <Input value={form.dbUser} onChange={(e) => set('dbUser', e.target.value)} />
                    </label>
                    <label className="flex flex-col gap-1.5">
                      <span className="text-xs font-medium text-muted-foreground">{t('project.dbPassword')}</span>
                      <Input type="password" autoComplete="new-password" value={form.dbPassword} onChange={(e) => set('dbPassword', e.target.value)} />
                    </label>
                    <label className="flex flex-col gap-1.5">
                      <span className="text-xs font-medium text-muted-foreground">{t('project.dbHost')}</span>
                      <Input value={form.dbHost} onChange={(e) => set('dbHost', e.target.value)} />
                    </label>
                    <label className="flex flex-col gap-1.5">
                      <span className="text-xs font-medium text-muted-foreground">{t('project.dbPort')}</span>
                      <Input inputMode="numeric" value={form.dbPort} placeholder="3306" onChange={(e) => set('dbPort', e.target.value)} />
                    </label>
                  </div>
                  <Switch checked={form.createDB} onChange={(v) => set('createDB', v)} label={t('project.createDB')} />
                  <Switch checked={form.startService} onChange={(v) => set('startService', v)} label={t('project.startService')} description={t('project.startServiceHelp')} />
                </>
              )}
            </div>
          )}
        </>
      )}
    </Dialog>
  );
}
