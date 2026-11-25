import type {
  DaemonClient,
  JobStatus,
  QueueInfo,
  ServiceHealth,
  SiteData,
  SiteStack,
  LairStatus,
  WebConfigPayload,
  WebOp,
  WebResult,
  ProjectCreatePayload,
  ProjectInfo,
  ProjectResult,
  NameList,
  MailPayload,
  MailResult,
  DumpsPayload,
  DumpsResult,
  DBResult,
  DebuggerPayload,
  DebuggerResult,
  LogSource,
  SitePayload,
  SiteResult,
} from './models';

export class DaemonError extends Error {
  constructor(
    message: string,
    readonly status: number,
  ) {
    super(message);
    this.name = 'DaemonError';
  }
}

interface Envelope<T> {
  ok?: boolean;
  data?: T;
  error?: string;
}

/** Upper bound for a single daemon bridge request (downloads via `bootstrap`
 * and catalog updates can legitimately take a few seconds). */
const REQUEST_TIMEOUT_MS = 30000;

/**
 * Network failures must never leak URLs, tokens or stack traces into the UI.
 * Only a small, stable vocabulary reaches the error banner.
 */
function networkError(cause: unknown): DaemonError {
  // DOMException (used for aborts/timeouts) is not always an Error subclass.
  const name =
    cause && typeof cause === 'object' && 'name' in cause
      ? String((cause as { name?: unknown }).name)
      : '';
  if (name === 'TimeoutError') return new DaemonError('request timed out', 0);
  if (name === 'AbortError') return new DaemonError('request canceled', 0);
  return new DaemonError('network error', 0);
}

/** Combines the caller's cancellation with the built-in request timeout. */
function requestSignal(signal?: AbortSignal): AbortSignal {
  const timeout = AbortSignal.timeout(REQUEST_TIMEOUT_MS);
  return signal ? AbortSignal.any([signal, timeout]) : timeout;
}

export const ACTIONS = {
  status: 'status',
  versions: 'versions',
  bootstrap: 'bootstrap',
  installPHP: 'install-php',
  installNode: 'install-node',
  uninstallPHP: 'php-uninstall',
  updateNode: 'node-update',
  uninstallNode: 'node-uninstall',
  updateComposer: 'composer-update',
  updateNpm: 'npm-update',
  retryJob: 'job-retry',
  installService: 'install-service',
  job: 'job',
  jobs: 'jobs',
  cancelJob: 'job-cancel',
  queue: 'queue',
  servicesHealth: 'services-health',
  siteData: 'site-data',
  siteStack: 'site-stack',
  worktree: 'worktree',
  log: 'logs',
  logsList: 'logs-list',
  mail: 'mail',
  dumps: 'dumps',
  debugger: 'debugger',
  usePHP: 'php-use',
  useNode: 'node-default',
  useService: 'service-use',
  restartService: 'service-restart',
  stopService: 'service-stop',
  uninstallService: 'service-uninstall',
  ensureDatabase: 'ensure-database',
  createDB: 'create-db',
  db: 'db',
  setDefaultDB: 'db-default',
  park: 'park',
  link: 'link',
  unlink: 'unlink',
  site: 'site',
  secure: 'secure',
  web: 'web',
  project: 'project',
  pin: 'pin',
  isolate: 'isolate',
  ensureShims: 'ensure-shims',
  restart: 'restart',
  updateCatalog: 'update',
  uninstall: 'uninstall',
  firstRun: 'first-run',
} as const;

/** HTTP client for the daemon's token-protected POST /api bridge. */
export class HttpDaemonClient implements DaemonClient {
  private readonly base: string;

  constructor(
    baseURL: string,
    private readonly token: string,
    private readonly fetchImpl: typeof fetch = globalThis.fetch,
    private readonly ipcCall?: (action: string, payload: unknown) => Promise<unknown>,
  ) {
    if (!token && !ipcCall) throw new DaemonError('missing daemon token', 401);
    this.base = baseURL.replace(/\/+$/, '');
  }

  async call<T>(action: string, payload: unknown = {}, signal?: AbortSignal): Promise<T> {
    if (this.ipcCall) {
      try {
        return (await this.ipcCall(action, payload)) as T;
      } catch (cause) {
        if (cause instanceof DaemonError) throw cause;
        throw new DaemonError(cause instanceof Error ? cause.message : 'daemon call failed', 0);
      }
    }
    let response: Response;
    // Call fetch detached: native fetch rejects a receiver that is not Window,
    // and calling it as `this.fetchImpl(...)` passes this client as receiver.
    const fetchImpl = this.fetchImpl;
    try {
      response = await fetchImpl(`${this.base}/api`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          'X-Lair-Token': this.token,
        },
        body: JSON.stringify({ action, payload }),
        signal: requestSignal(signal),
      });
    } catch (cause) {
      throw networkError(cause);
    }

    if (!response.ok) {
      // The daemon's bridge returns { ok:false, error:"..." }. Surface that
      // detail instead of a generic status so the user sees the real cause
      // (for example an unreachable catalog mirror).
      let detail = '';
      try {
        const body = (await response.json()) as Envelope<T>;
        detail = typeof body.error === 'string' ? body.error.trim() : '';
      } catch {
        // A proxy or a crashed daemon may return a non-JSON body.
      }
      throw new DaemonError(detail || `daemon responded ${response.status}`, response.status);
    }
    let body: Envelope<T>;
    try {
      body = (await response.json()) as Envelope<T>;
    } catch {
      throw new DaemonError('invalid response from daemon', response.status);
    }
    if (!body.ok) {
      throw new DaemonError(body.error || 'daemon call failed', response.status);
    }
    // A daemon action that returns nil leaves `data` out; an empty object keeps
    // the decoders below tolerant instead of throwing on undefined.
    return (body.data ?? {}) as T;
  }

  status(signal?: AbortSignal) {
    return this.call<LairStatus>(ACTIONS.status, {}, signal);
  }

  async versions(packageName: string) {
    const result = await this.call<{ versions?: string[] }>(ACTIONS.versions, {
      package: packageName,
    });
    return result.versions ?? [];
  }

  installComposer() {
    return this.call<{ composer_path?: string; bin_dir?: string; warnings?: string[] }>(ACTIONS.bootstrap, {});
  }

  installPHP(version: string) {
    return this.call<JobStatus>(ACTIONS.installPHP, { version });
  }

  installNode(version: string) {
    return this.call<JobStatus>(ACTIONS.installNode, { version });
  }

  async uninstallPHP(minor: string) {
    await this.call(ACTIONS.uninstallPHP, { minor });
  }

  updateNode(major: string) {
    return this.call<JobStatus>(ACTIONS.updateNode, { version: major });
  }

  async uninstallNode(major: string) {
    await this.call(ACTIONS.uninstallNode, { version: major });
  }

  updateComposer() {
    return this.call<{ text?: string }>(ACTIONS.updateComposer);
  }

  updateNpm() {
    return this.call<{ text?: string }>(ACTIONS.updateNpm);
  }

  retryJob(id: string) {
    return this.call<JobStatus>(ACTIONS.retryJob, { id });
  }

  installService(id: string, version: string, port?: number) {
    return this.call<JobStatus>(ACTIONS.installService, { id, version, port });
  }

  job(id: string, signal?: AbortSignal) {
    return this.call<JobStatus>(ACTIONS.job, { id }, signal);
  }

  jobs() {
    return this.call<JobStatus[]>(ACTIONS.jobs);
  }

  queue(name: string) {
    return this.call<QueueInfo>(ACTIONS.queue, { name });
  }

  servicesHealth() {
    return this.call<ServiceHealth[]>(ACTIONS.servicesHealth);
  }

  siteData(name: string) {
    return this.call<SiteData>(ACTIONS.siteData, { name });
  }

  siteStack(name: string) {
    return this.call<SiteStack>(ACTIONS.siteStack, { name });
  }

  cancelJob(id: string) {
    return this.call<{ canceled?: boolean }>(ACTIONS.cancelJob, { id });
  }

  async log(name: string) {
    const result = await this.call<{ text?: string }>(ACTIONS.log, { name });
    return result.text ?? '';
  }

  async logsList() {
    const result = await this.call<NameList>(ACTIONS.logsList);
    return result.items ?? [];
  }

  async logSources(): Promise<LogSource[]> {
    const result = await this.call<NameList>(ACTIONS.logsList);
    return result.sources ?? [];
  }

  mail(payload: MailPayload) {
    return this.call<MailResult>(ACTIONS.mail, payload);
  }

  mailRaw(id: string, inbox?: string) {
    return this.call<MailResult>(ACTIONS.mail, { op: 'raw', id, inbox }).then((r) => r.raw ?? '');
  }

  dumps(payload: DumpsPayload) {
    return this.call<DumpsResult>(ACTIONS.dumps, payload);
  }

  debugger(payload: DebuggerPayload) {
    return this.call<DebuggerResult>(ACTIONS.debugger, payload);
  }

  async usePHP(minor: string) {
    await this.call(ACTIONS.usePHP, { minor });
  }

  async useNode(version: string) {
    await this.call(ACTIONS.useNode, { version });
  }

  async useService(id: string, version: string) {
    await this.call(ACTIONS.useService, { id, version });
  }

  async restartService(id: string) {
    await this.call(ACTIONS.restartService, { id });
  }

  async stopService(id: string) {
    await this.call(ACTIONS.stopService, { id });
  }

  async uninstallService(id: string, version: string, deleteData = false) {
    await this.call(ACTIONS.uninstallService, { id, version, delete_data: deleteData });
  }

  async ensureDatabase() {
    return this.call<JobStatus & { already_installed?: boolean }>(ACTIONS.ensureDatabase);
  }

  async createDB(id: string, name: string) {
    const result = await this.call<{ text?: string }>(ACTIONS.createDB, { id, name });
    return result.text ?? '';
  }

  async db(payload: Record<string, unknown>) {
    return this.call<DBResult>(ACTIONS.db, payload);
  }

  async setDefaultDB(id: string) {
    await this.call(ACTIONS.setDefaultDB, { id });
  }

  async park(path: string) {
    const result = await this.call<{ text?: string }>(ACTIONS.park, { path });
    return result.text ?? path;
  }

  async link(name: string, path: string) {
    await this.call(ACTIONS.link, { name, path });
  }

  async unlink(name: string, deleteFiles = false) {
    await this.call(ACTIONS.unlink, { name, delete_files: deleteFiles });
  }

  site(payload: SitePayload) {
    return this.call<SiteResult>(ACTIONS.site, payload);
  }

  siteSetEnabled(name: string, enabled: boolean) {
    return this.call<SiteResult>(ACTIONS.site, { op: 'set-enabled', name, enabled });
  }

  siteConfig(name: string) {
    return this.call<SiteResult>(ACTIONS.site, { op: 'config', name });
  }

  siteOpen(name: string) {
    return this.call<SiteResult>(ACTIONS.site, { op: 'open', name });
  }

  async secure(name: string, on = true) {
    await this.call(ACTIONS.secure, { name, on });
  }

  web(op: WebOp, payload: WebConfigPayload = {}) {
    return this.call<WebResult>(ACTIONS.web, { op, ...payload });
  }

  async projectInfo(): Promise<ProjectInfo> {
    // The daemon action returns { info: ProjectInfo }; FakeDaemon returns the
    // object directly. Accept both so the wizard sees the real defaults.
    const result = await this.call<ProjectInfo & { info?: ProjectInfo }>(ACTIONS.project, { op: 'info' });
    return result.info ?? result;
  }

  projectCreate(payload: ProjectCreatePayload) {
    return this.call<ProjectResult>(ACTIONS.project, { op: 'create', ...payload });
  }

  async projectSetDB(name: string, database: string) {
    await this.call(ACTIONS.project, { op: 'set-db', name, database });
  }

  async projectSync() {
    await this.call(ACTIONS.project, { op: 'sync' });
  }

  projectDirs() {
    return this.call<ProjectResult>(ACTIONS.project, { op: 'dirs' });
  }

  projectDirsAdd(dir: string) {
    return this.call<ProjectResult>(ACTIONS.project, { op: 'dirs-add', dir });
  }

  projectDirsRemove(dir: string) {
    return this.call<ProjectResult>(ACTIONS.project, { op: 'dirs-remove', dir });
  }

  projectMigrateDirs(from: string, to: string) {
    return this.call<ProjectResult>(ACTIONS.project, { op: 'migrate-dirs', from, to });
  }

  projectImportValet() {
    return this.call<ProjectResult>(ACTIONS.project, { op: 'import-valet' });
  }

  async pin(tool: string, value: string, dir?: string) {
    await this.call(ACTIONS.pin, { tool, value, dir });
  }

  async isolate(tool: string, value: string, dir?: string) {
    await this.call(ACTIONS.isolate, { tool, value, dir });
  }

  async ensureShims(signal?: AbortSignal) {
    await this.call(ACTIONS.ensureShims, {}, signal);
  }

  async restart() {
    await this.call(ACTIONS.restart);
  }

  async updateCatalog() {
    const result = await this.call<{ text?: string }>(ACTIONS.updateCatalog);
    return result.text ?? '';
  }

  async uninstall(deleteData = false) {
    await this.call(ACTIONS.uninstall, { delete_data: deleteData });
  }

  firstRun() {
    return this.call<{ jobs?: JobStatus[] }>(ACTIONS.firstRun);
  }
}

/** Reads the connection details published by the daemon to the main process. */
export async function connectFromDesktop(
  fetchImpl?: typeof fetch,
): Promise<HttpDaemonClient | null> {
  const bridge = window.lairDesktop;
  if (!bridge) return null;
  const details = await bridge.connect();
  if (!details) return null;
  if (typeof bridge.call === 'function') {
    return new HttpDaemonClient(details.url, '', fetchImpl, (action, payload) =>
      bridge.call!(action, payload),
    );
  }
  // Browser/dev fallback (no Electron bridge): token is only used here.
  const legacy = details as { url: string; token?: string };
  return new HttpDaemonClient(details.url, legacy.token ?? '', fetchImpl);
}
