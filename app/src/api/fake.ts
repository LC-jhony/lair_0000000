import demo from './fixtures/demo.json';
import { ACTIONS } from './client';
import type {
  DaemonClient,
  JobStatus,
  LairStatus,
  WebConfigPayload,
  WebOp,
  WebResult,
  ProjectCreatePayload,
  ProjectInfo,
  ProjectResult,
  MailPayload,
  MailResult,
  DumpsPayload,
  DumpsResult,
  DebuggerPayload,
  DebuggerResult,
  LogSource,
  SiteDetail,
  SiteEnvEntry,
  SitePayload,
  SiteResult,
} from './models';

interface FixtureData {
  status: LairStatus;
  versions: Record<string, string[]>;
  logs: Record<string, string>;
  mail?: MailResult;
  dumps?: DumpsResult;
  debugger?: DebuggerResult;
  sources?: LogSource[];
  /** Initial state of the `project dirs` operations. */
  project?: { dirs?: string[] };
}

/** `~/Lair` as represented in the demo fixture (home is /home/dev). */
export const DEFAULT_PROJECT_DIR = '/home/dev/Lair';
/** Legacy lowercase folder used by `migrate-dirs` in the demo fixture. */
export const LEGACY_PROJECT_DIR = '/home/dev/lair';

function clone<T>(value: T): T {
  return JSON.parse(JSON.stringify(value)) as T;
}

export interface FakeOptions {
  fixture?: FixtureData;
  /** Job ids that must fail on the first advance, for failure-state captures. */
  failOn?: string[];
}

/** In-process daemon used for UI development and reproducible screenshots. */
export class FakeDaemon implements DaemonClient {
  private readonly statusData: LairStatus;
  private readonly versionsData: Record<string, string[]>;
  private readonly logsData: Record<string, string>;
  private readonly jobsData = new Map<string, JobStatus>();
  private readonly failOn: Set<string>;
  private webState: WebResult['web'];
  private mailState: MailResult;
  private dumpsState: DumpsResult;
  private debuggerState: DebuggerResult;
  private readonly sourcesData: LogSource[];
  private readonly siteEnv = new Map<string, SiteEnvEntry[]>();
  private readonly siteDomains = new Map<string, string[]>();
  private readonly siteEnabled = new Map<string, boolean>();
  /** Mutable project folder list shared by all `project dirs` operations. */
  private projectDirsState: string[];
  /** Last `unlink` call, so tests can assert the delete-files flag. */
  lastUnlink?: { name: string; deleteFiles: boolean };

  constructor(options: FakeOptions = {}) {
    const fixture = options.fixture ?? (demo as FixtureData);
    this.statusData = clone(fixture.status);
    this.versionsData = clone(fixture.versions);
    this.logsData = clone(fixture.logs);
    this.failOn = new Set(options.failOn ?? []);
    this.webState = clone(this.statusData.web ?? {});
    this.mailState = clone(fixture.mail ?? {});
    this.dumpsState = clone(fixture.dumps ?? {});
    this.debuggerState = clone(fixture.debugger ?? {});
    this.sourcesData = clone(fixture.sources ?? []);
    const initialDirs = fixture.project?.dirs;
    this.projectDirsState = (initialDirs?.length ? initialDirs : [DEFAULT_PROJECT_DIR]).map(String);
  }

  async call<T>(action: string, payload: unknown = {}): Promise<T> {
    const p = (payload ?? {}) as Record<string, unknown>;
    switch (action) {
      case ACTIONS.status:
        return clone(this.statusData) as T;
      case ACTIONS.versions:
        return { versions: this.versionsData[String(p.package)] ?? [] } as T;
      case ACTIONS.bootstrap:
        return (await this.installComposer()) as T;
      case ACTIONS.installPHP:
        return (await this.installPHP(String(p.version))) as T;
      case ACTIONS.installNode:
        return (await this.installNode(String(p.version))) as T;
      case ACTIONS.uninstallPHP:
        await this.uninstallPHP(String(p.minor));
        return undefined as T;
      case ACTIONS.updateNode:
        return (await this.updateNode(String(p.version))) as T;
      case ACTIONS.uninstallNode:
        await this.uninstallNode(String(p.version));
        return undefined as T;
      case ACTIONS.updateComposer:
        return (await this.updateComposer()) as T;
      case ACTIONS.updateNpm:
        return (await this.updateNpm()) as T;
      case ACTIONS.retryJob:
        return this.retryJob(String(p.id)) as T;
      case ACTIONS.installService:
        return (await this.installService(
          String(p.id),
          String(p.version),
          Number(p.port) || undefined,
        )) as T;
      case ACTIONS.job:
        return this.job(String(p.id)) as T;
      case ACTIONS.jobs:
        return this.jobs() as T;
      case ACTIONS.cancelJob:
        return this.cancelJob(String(p.id)) as T;
      case ACTIONS.log:
        return { text: await this.log(String(p.name)) } as T;
      case ACTIONS.logsList:
        return { items: await this.logsList(), sources: clone(this.sourcesData) } as T;
      case ACTIONS.mail:
        return this.mail(p as unknown as MailPayload) as T;
      case ACTIONS.dumps:
        return this.dumps(p as unknown as DumpsPayload) as T;
      case ACTIONS.debugger:
        return this.debugger(p as unknown as DebuggerPayload) as T;
      case ACTIONS.usePHP:
        await this.usePHP(String(p.minor));
        return undefined as T;
      case ACTIONS.useNode:
        await this.useNode(String(p.version));
        return undefined as T;
      case ACTIONS.useService:
        await this.useService(String(p.id), String(p.version));
        return undefined as T;
      case ACTIONS.restartService:
        await this.restartService(String(p.id));
        return undefined as T;
      case ACTIONS.stopService:
        await this.stopService(String(p.id));
        return undefined as T;
      case ACTIONS.uninstallService:
        await this.uninstallService(String(p.id), String(p.version), Boolean(p.delete_data));
        return undefined as T;
      case ACTIONS.setDefaultDB:
        await this.setDefaultDB(String(p.id));
        return undefined as T;
      case ACTIONS.ensureDatabase:
        return { already_installed: true } as T;
      case ACTIONS.createDB:
        return { text: await this.createDB(String(p.id), String(p.name)) } as T;
      case ACTIONS.park:
        return { text: await this.park(String(p.path)) } as T;
      case ACTIONS.link:
        await this.link(String(p.name), String(p.path));
        return undefined as T;
      case ACTIONS.unlink:
        await this.unlink(String(p.name), Boolean(p.delete_files));
        return undefined as T;
      case ACTIONS.site:
        return this.site(p as unknown as SitePayload) as T;
      case ACTIONS.secure:
        await this.secure(String(p.name), p.on === undefined ? true : Boolean(p.on));
        return undefined as T;
      case ACTIONS.web:
        return this.web(String(p.op) as WebOp, (p.payload ?? p) as WebConfigPayload) as T;
      case ACTIONS.project:
        if (p.op === 'create') {
          return (await this.projectCreate(p as unknown as ProjectCreatePayload)) as T;
        }
        if (p.op === 'set-db') {
          await this.projectSetDB(String(p.name), String(p.database));
          return undefined as T;
        }
        if (p.op === 'sync') {
          await this.projectSync();
          return undefined as T;
        }
        if (p.op === 'dirs') {
          return this.projectDirs() as T;
        }
        if (p.op === 'dirs-add') {
          return this.projectDirsAdd(String(p.dir ?? '')) as T;
        }
        if (p.op === 'dirs-remove') {
          return this.projectDirsRemove(String(p.dir ?? '')) as T;
        }
        if (p.op === 'migrate-dirs') {
          return this.projectMigrateDirs(String(p.from ?? ''), String(p.to ?? '')) as T;
        }
        if (p.op === 'import-valet') {
          return this.projectImportValet() as T;
        }
        return this.projectInfo() as T;
      case ACTIONS.pin:
        await this.pin(String(p.tool), String(p.value));
        return undefined as T;
      case ACTIONS.isolate:
        await this.isolate(String(p.tool), String(p.value), String(p.dir ?? p.name ?? ''));
        return undefined as T;
      case ACTIONS.ensureShims:
        return undefined as T;
      case ACTIONS.restart:
        return undefined as T;
      case ACTIONS.updateCatalog:
        return { text: await this.updateCatalog() } as T;
      default:
        throw new Error(`unknown action: ${action}`);
    }
  }

  async status() {
    return clone(this.statusData);
  }

  async versions(packageName: string) {
    return [...(this.versionsData[packageName] ?? [])];
  }

  async installComposer() {
    this.statusData.composer = this.versionsData.composer?.[0] ?? '2.11.2';
    return { composer_path: '/home/dev/.local/share/lair/tools/composer.phar', warnings: [] };
  }

  async installPHP(version: string) {
    return this.startJob(`php/${version}`, version);
  }

  async installNode(version: string) {
    return this.startJob(`node/${version}`, version);
  }

  async installService(id: string, version: string, _port?: number) {
    return this.startJob(`${id}/${version}`, version);
  }

  async job(id: string) {
    return clone(this.jobsData.get(id) ?? { id, done: true });
  }

  async jobs() {
    return [...this.jobsData.values()].filter((job) => !job.done).map((job) => clone(job));
  }

  async cancelJob(id: string) {
    const job = this.jobsData.get(id);
    if (job) {
      job.canceled = true;
      job.error = 'canceled';
      job.done = true;
    }
    return { canceled: true };
  }

  async log(name: string) {
    return this.logsData[name] ?? '';
  }

  async usePHP(minor: string) {
    if (this.statusData.defaults) this.statusData.defaults.php = minor;
  }

  async useNode(version: string) {
    if (this.statusData.defaults) this.statusData.defaults.node = version;
  }

  async uninstallPHP(minor: string) {
    this.statusData.php = (this.statusData.php ?? []).filter((info) => (info.minor ?? info.version) !== minor);
  }

  async updateNode(major: string) {
    return this.installNode(major);
  }

  async uninstallNode(major: string) {
    this.statusData.node = (this.statusData.node ?? []).filter((v) => v.split('.')[0] !== major.replace(/^v/, ''));
  }

  async updateComposer() {
    return { text: 'tools/composer.phar' };
  }

  async updateNpm() {
    return { text: '10.9.0' };
  }

  async retryJob(id: string) {
    return this.job(id);
  }

  async useService(id: string, version: string) {
    const service = this.statusData.services?.[id];
    if (service) {
      service.installed = version;
      service.version = version;
      service.running = true;
    }
  }

  async restartService(id: string) {
    const service = this.statusData.services?.[id];
    if (service) service.running = true;
  }

  async stopService(id: string) {
    const service = this.statusData.services?.[id];
    if (service) service.running = false;
  }

  async uninstallService(id: string, _version: string, deleteData = false) {
    const service = this.statusData.services?.[id];
    if (service) {
      service.state = 'missing';
      service.running = false;
      if (deleteData) delete service.installed;
    }
  }

  async setDefaultDB(id: string) {
    if (this.statusData.defaults) this.statusData.defaults.database = id;
  }

  async updateCatalog() {
    // Fixtures are already current, so there is nothing to refresh.
    return 'El catálogo firmado ya está actualizado.';
  }

  async firstRun() {
    return { jobs: [] };
  }

  async uninstall(_deleteData = false) {
    this.statusData.php = [];
    this.statusData.node = [];
    this.statusData.composer = undefined;
    this.statusData.services = {};
    this.statusData.sites = [];
  }

  async logsList() {
    return Object.keys(this.logsData).sort();
  }

  async logSources() {
    return clone(this.sourcesData);
  }

  private syncToolingStatus() {
    this.statusData.mail_count = this.mailState.messages?.length ?? this.mailState.count ?? 0;
    this.statusData.dump_count = this.dumpsState.events?.length ?? this.dumpsState.count ?? 0;
    this.statusData.debug_running = Boolean(this.debuggerState.detect || this.debuggerState.breakpoints);
  }

  private mailSnapshot(output?: string): MailResult {
    const messages = clone(this.mailState.messages ?? []);
    return {
      ...clone(this.mailState),
      count: messages.length,
      messages,
      ...(output ? { output } : {}),
    };
  }

  async mail(payload: MailPayload): Promise<MailResult> {
    switch (payload.op) {
      case 'start':
        this.mailState.running = true;
        this.syncToolingStatus();
        return this.mailSnapshot(`smtp listening on ${this.mailState.port ?? 2525}`);
      case 'stop':
        this.mailState.running = false;
        this.syncToolingStatus();
        return this.mailSnapshot('smtp stopped');
      case 'clear':
        this.mailState.messages = [];
        this.syncToolingStatus();
        return this.mailSnapshot('inbox cleared');
      case 'message': {
        const found = (this.mailState.messages ?? []).find((message) => message.id === payload.id) ?? null;
        return { ...this.mailSnapshot(), message: found ? clone(found) : null };
      }
      case 'list':
        return this.mailSnapshot();
      case 'status':
      default:
        return this.mailSnapshot();
    }
  }

  async mailRaw(_id: string, _inbox?: string): Promise<string> {
    return '';
  }

  private dumpsSnapshot(output?: string): DumpsResult {
    const events = clone(this.dumpsState.events ?? []);
    return {
      ...clone(this.dumpsState),
      options: { ...(this.dumpsState.options ?? {}) },
      count: events.length,
      events,
      ...(output ? { output } : {}),
    };
  }

  async dumps(payload: DumpsPayload): Promise<DumpsResult> {
    switch (payload.op) {
      case 'capture-on':
        this.dumpsState.capturing = true;
        return this.dumpsSnapshot('dump capture enabled');
      case 'capture-off':
        this.dumpsState.capturing = false;
        return this.dumpsSnapshot('dump capture disabled');
      case 'options':
        this.dumpsState.options = {
          ...(this.dumpsState.options ?? {}),
          ...(payload.types ? {
            queries: payload.types.includes('queries'),
            logs: payload.types.includes('logs'),
            views: payload.types.includes('views'),
            jobs: payload.types.includes('jobs'),
            requests: payload.types.includes('requests'),
          } : {}),
          ...(payload.show_in_page === undefined ? {} : { show_in_page: payload.show_in_page }),
        };
        return this.dumpsSnapshot('dump options updated');
      case 'clear':
        this.dumpsState.events = [];
        this.syncToolingStatus();
        return this.dumpsSnapshot('dumps cleared');
      case 'list':
      case 'status':
      default:
        return this.dumpsSnapshot();
    }
  }

  private debuggerSnapshot(output?: string): DebuggerResult {
    return {
      ...clone(this.debuggerState),
      versions: clone(this.debuggerState.versions ?? []),
      domains: clone(this.debuggerState.domains ?? []),
      ...(output ? { output } : {}),
    };
  }

  async debugger(payload: DebuggerPayload): Promise<DebuggerResult> {
    switch (payload.op) {
      case 'detect-on':
        this.debuggerState.detect = true;
        this.syncToolingStatus();
        return this.debuggerSnapshot('xdebug request detection enabled');
      case 'detect-off':
        this.debuggerState.detect = false;
        this.syncToolingStatus();
        return this.debuggerSnapshot('xdebug request detection disabled');
      case 'breakpoints-on':
        this.debuggerState.breakpoints = true;
        this.syncToolingStatus();
        return this.debuggerSnapshot('PhpStorm breakpoint detection enabled');
      case 'breakpoints-off':
        this.debuggerState.breakpoints = false;
        this.syncToolingStatus();
        return this.debuggerSnapshot('PhpStorm breakpoint detection disabled');
      case 'status':
      default:
        return this.debuggerSnapshot();
    }
  }

  async ensureDatabase() {
    return { already_installed: true } as JobStatus & { already_installed?: boolean };
  }

  async createDB(id: string, name: string) {
    return `CREATE DATABASE ${name} (${id})`;
  }

  async park(path: string) {
    const name = `${path.split('/').filter(Boolean).pop() ?? 'site'}.test`;
    this.statusData.sites = [...(this.statusData.sites ?? []), { name, path }];
    return name;
  }

  async link(name: string, path: string) {
    this.statusData.sites = [...(this.statusData.sites ?? []), { name, path }];
  }

  async unlink(name: string, deleteFiles = false) {
    this.lastUnlink = { name, deleteFiles };
    this.statusData.sites = (this.statusData.sites ?? []).filter((s) => s.name !== name);
  }

  private siteOf(name: string) {
    return (this.statusData.sites ?? []).find((s) => (s.name ?? s.host) === name);
  }

  private defaultEnv(name: string): SiteEnvEntry[] {
    const host = name || 'demo.test';
    const short = host.split('.')[0] || 'demo';
    const database = this.siteOf(host)?.database ?? 'mariadb';
    return [
      { key: 'APP_NAME', value: short, secret: false, set: true },
      { key: 'APP_ENV', value: 'local', secret: false, set: true },
      { key: 'APP_KEY', value: 'base64:demo-secret-key-do-not-show', secret: true, set: true },
      { key: 'APP_DEBUG', value: 'true', secret: false, set: true },
      { key: 'APP_URL', value: `https://${host}`, secret: false, set: true },
      { key: 'DB_CONNECTION', value: database, secret: false, set: true },
      { key: 'DB_HOST', value: '127.0.0.1', secret: false, set: true },
      { key: 'DB_PORT', value: database === 'pgsql' ? '5432' : '3306', secret: false, set: true },
      { key: 'DB_DATABASE', value: `${short}_dev`, secret: false, set: true },
      { key: 'DB_USERNAME', value: short, secret: false, set: true },
      { key: 'DB_PASSWORD', value: 'demo-db-secret', secret: true, set: true },
      { key: 'MAIL_MAILER', value: 'smtp', secret: false, set: true },
      { key: 'MAIL_HOST', value: '127.0.0.1', secret: false, set: true },
      { key: 'MAIL_PORT', value: String(this.mailState.port ?? 2525), secret: false, set: true },
    ];
  }

  private siteDetail(name: string): SiteDetail {
    const site = this.siteOf(name) ?? {};
    const host = site.name ?? site.host ?? name;
    const root = site.root ?? site.path ?? '';
    const web = this.webState ?? {};
    const secure = Boolean(site.secure);
    const enabled = this.siteEnabled.get(host) ?? site.enabled ?? true;
    const port = secure ? (web.https_port ?? 8443) : (web.http_port ?? 8080);
    const authority = port === 80 || port === 443 ? host : `${host}:${port}`;
    const url = String(site.url ?? `${secure ? 'https' : 'http'}://${authority}`);
    const php = String(site.php ?? this.statusData.defaults?.php ?? '8.4');
    const node = String(site.node ?? this.statusData.defaults?.node ?? 'v22.16.0');
    const database = String(site.database ?? this.statusData.defaults?.database ?? 'sqlite');
    const dbPort = database === 'pgsql' ? 5432 : database === 'sqlite' ? undefined : 3306;
    return {
      name: host,
      path: root,
      url,
      type: 'laravel',
      framework: 'Laravel',
      php,
      php_version: php,
      php_source: site.php ? 'isolated' : 'default',
      php_file: root ? `${root}/.php-version` : '',
      node,
      node_source: site.node ? 'isolated' : 'default',
      node_file: root ? `${root}/.nvmrc` : '',
      database,
      database_host: database === 'sqlite' ? '' : '127.0.0.1',
      ...(dbPort ? { database_port: dbPort } : {}),
      database_name: `${(host.split('.')[0] || 'demo').replace(/-/g, '_')}_dev`,
      secure,
      enabled,
      root,
      root_state: String(site.root_state ?? (root ? 'ok' : 'missing')),
      root_suggestions: root ? [] : [`${LEGACY_PROJECT_DIR}/${host.split('.')[0] || 'demo'}`],
      domains: this.siteDomains.get(host) ?? [host],
      problem: '',
      notice: '',
    };
  }

  /** A small, deterministic vhost sample for the inspector preview. */
  private siteVhost(name: string): string {
    const detail = this.siteDetail(name);
    const host = detail.name ?? name;
    const domains = [...new Set([host, ...(detail.domains ?? [])])].join(' ');
    const port = detail.secure
      ? (this.webState?.https_port ?? 8443)
      : (this.webState?.http_port ?? 8080);
    const root = detail.root || `/srv/${host}`;
    const socket = `/home/dev/.lair/run/php-${String(detail.php ?? '8.4').replace(/[^0-9.]/g, '')}-fpm.sock`;
    return [
      '# generated by lair (demo)',
      `# site ${host} · ${detail.enabled ? 'enabled' : 'disabled'}`,
      'server {',
      `    listen 127.0.0.1:${port}${detail.secure ? ' ssl' : ''};`,
      `    server_name ${domains};`,
      `    root ${root};`,
      '',
      '    location / {',
      `        try_files $uri $uri/ /index.php?$query_string;`,
      '    }',
      '',
      '    location ~ \\.php$ {',
      `        fastcgi_pass unix:${socket};`,
      '        include fastcgi_params;',
      '    }',
      '}',
    ].join('\n');
  }

  async site(payload: SitePayload): Promise<SiteResult> {
    const name = String(payload.name ?? '');
    switch (payload.op) {
      case 'detail':
        return { detail: this.siteDetail(name) };
      case 'env': {
        const entries = this.siteEnv.get(name) ?? this.defaultEnv(name);
        this.siteEnv.set(name, clone(entries));
        return { env: clone(entries) };
      }
      case 'env-set': {
        const entries = this.siteEnv.get(name) ?? this.defaultEnv(name);
        const key = String(payload.key ?? '');
        const existing = entries.find((entry) => entry.key === key);
        if (existing) {
          existing.value = String(payload.value ?? '');
          existing.set = true;
        } else {
          entries.push({ key, value: String(payload.value ?? ''), secret: false, set: true });
        }
        this.siteEnv.set(name, clone(entries));
        return { env: clone(entries), output: `${key}=updated` };
      }
      case 'domains': {
        const found: unknown = this.siteDomains.get(name) ?? this.siteOf(name)?.domains ?? [name];
        const domains = Array.isArray(found) ? found.filter((d): d is string => typeof d === 'string') : [name];
        return { domains: clone(domains) };
      }
      case 'delete':
        await this.unlink(name, Boolean(payload.delete_files));
        return { output: `${name} removed` };
      case 'set-enabled': {
        const site = this.siteOf(name) ?? {};
        const host = String(site.name ?? site.host ?? name);
        const enabled = Boolean(payload.enabled);
        this.siteEnabled.set(host, enabled);
        const registered = (this.statusData.sites ?? []).find((item) => (item.name ?? item.host) === host);
        if (registered) registered.enabled = enabled;
        return {
          detail: this.siteDetail(name),
          output: `${host} ${enabled ? 'enabled' : 'disabled'}`,
        };
      }
      case 'config':
        // The generated vhost is readable even when the site is disabled.
        return { config: this.siteVhost(name) };
      case 'open': {
        const site = this.siteOf(name) ?? {};
        const host = String(site.name ?? site.host ?? name);
        const web = this.webState ?? {};
        const secure = Boolean(site.secure && web.https);
        const port = secure ? (web.https_port ?? 443) : (web.http_port ?? 80);
        const authority = port === 80 || port === 443 ? host : `${host}:${port}`;
        const url = `${secure ? 'https' : 'http'}://${authority}`;
        return { url, output: url };
      }
      default:
        return {};
    }
  }

  async siteSetEnabled(name: string, enabled: boolean): Promise<SiteResult> {
    return this.site({ op: 'set-enabled', name, enabled });
  }

  async siteConfig(name: string): Promise<SiteResult> {
    return this.site({ op: 'config', name });
  }

  async siteOpen(name: string): Promise<SiteResult> {
    return this.site({ op: 'open', name });
  }

  async secure(name: string, on = true) {
    const site = (this.statusData.sites ?? []).find((s) => s.name === name);
    if (site) site.secure = on;
  }

  async web(op: WebOp, payload: WebConfigPayload = {}): Promise<WebResult> {
    const current = this.webState ?? {};
    switch (op) {
      case 'configure':
      case 'https-on':
      case 'https-off': {
        this.webState = {
          ...current,
          http_port: payload.http_port ?? current.http_port ?? 80,
          https_port: payload.https_port ?? current.https_port ?? 443,
          https: op === 'https-on' ? true : op === 'https-off' ? false : payload.https ?? current.https ?? true,
          redirect_http: op === 'https-on' ? true : payload.redirect_http ?? current.redirect_http ?? true,
          bind: payload.bind ?? current.bind ?? '127.0.0.1',
          domain: payload.domain ?? current.domain ?? 'test',
        };
        this.statusData.web = clone(this.webState);
        return { web: clone(this.webState) };
      }
      case 'status':
        return { web: clone(this.webState) };
      default:
        throw new Error(`unknown web operation: ${String(op)}`);
    }
  }

  async projectInfo(): Promise<ProjectInfo> {
    return {
      laravel_installed: true,
      laravel_version: 'Laravel Installer 5.9.0',
      php: ['8.3', '8.4'],
      node: ['v22.16.0'],
      pdo_drivers: ['sqlite', 'mysql', 'pgsql'],
      node_managers: ['npm'],
      databases: ['sqlite', 'mysql', 'mariadb', 'pgsql'],
      projects_dir: this.projectDirsState[0] ?? DEFAULT_PROJECT_DIR,
      hint: '',
    };
  }

  async projectDirs(): Promise<ProjectResult> {
    return { dirs: [...this.projectDirsState] };
  }

  async projectDirsAdd(dir: string): Promise<ProjectResult> {
    const value = String(dir ?? '').trim();
    if (value && !this.projectDirsState.includes(value)) this.projectDirsState.push(value);
    return {
      dirs: [...this.projectDirsState],
      output: value ? `added ${value}` : '',
    };
  }

  async projectDirsRemove(dir: string): Promise<ProjectResult> {
    const value = String(dir ?? '').trim();
    const before = this.projectDirsState.length;
    this.projectDirsState = this.projectDirsState.filter((item) => item !== value);
    return {
      dirs: [...this.projectDirsState],
      output: this.projectDirsState.length < before ? `removed ${value}` : `${value} is not configured`,
    };
  }

  async projectMigrateDirs(from: string, to: string): Promise<ProjectResult> {
    const source = String(from ?? '').trim();
    const target = String(to ?? '').trim();
    if (source && target && source !== target) {
      const index = this.projectDirsState.indexOf(source);
      if (index >= 0) {
        if (!this.projectDirsState.includes(target)) this.projectDirsState[index] = target;
        else this.projectDirsState.splice(index, 1);
      } else if (!this.projectDirsState.includes(target)) {
        this.projectDirsState.push(target);
      }
    }
    return {
      dirs: [...this.projectDirsState],
      output: `migrated ${source} to ${target}`,
    };
  }

  async projectImportValet(): Promise<ProjectResult> {
    // Valet keeps its sites in ~/.valet/Sites; importing merges it into Lair.
    const valetDir = '/home/dev/.valet/Sites';
    if (!this.projectDirsState.includes(valetDir)) this.projectDirsState.push(valetDir);
    return {
      dirs: [...this.projectDirsState],
      output: `Imported Valet settings: ${valetDir}`,
    };
  }

  async projectCreate(payload: ProjectCreatePayload): Promise<ProjectResult> {
    const id = `project-${payload.name}`;
    this.logsData[id] = `laravel new ${payload.name} --database=${payload.database}\n`;
    const job = this.startJob(id, `project ${payload.name}`);
    this.statusData.sites = [
      ...(this.statusData.sites ?? []),
      {
        name: `${payload.name}.test`,
        root: `${payload.parent}/${payload.name}`,
        path: `${payload.parent}/${payload.name}`,
        secure: payload.secure,
        database: payload.database,
      },
    ];
    return { ...job, site: `${payload.name}.test`, path: `${payload.parent}/${payload.name}` };
  }

  async projectSetDB(name: string, database: string) {
    const site = (this.statusData.sites ?? []).find((s) => s.name === name);
    if (site) site.database = database;
  }

  async projectSync() {
    // Fixtures are already in sync.
  }

  async pin(tool: string, value: string) {
    if (!this.statusData.defaults) this.statusData.defaults = {};
    this.statusData.defaults[tool] = value;
  }

  async isolate(tool: string, value: string, dir?: string) {
    if (!this.statusData.defaults) this.statusData.defaults = {};
    this.statusData.defaults[tool] = value;
    if (dir) this.logsData[`isolate:${dir}`] = `${tool}=${value}\n`;
  }

  async ensureShims() {
    // The fake daemon has no filesystem side effects.
  }

  async restart() {
    // The fake daemon has no supervisors to restart.
  }

  private startJob(id: string, version: string): JobStatus {
    const job: JobStatus = {
      id,
      phase: 'downloading',
      downloaded: 0,
      total: 100,
      done: false,
    };
    this.jobsData.set(id, job);
    this.logsData[id] = `install ${version}\n`;
    return clone(job);
  }

  /** Advances every running job one step (25%). Used by tests and captures. */
  advance(step = 25) {
    for (const job of this.jobsData.values()) {
      if (job.done || job.phase === 'installing') continue;
      if (this.failOn.has(job.id ?? '')) {
        job.error = 'checksum mismatch (simulated)';
        job.done = true;
        continue;
      }
      job.downloaded = Math.min(job.total ?? 100, (job.downloaded ?? 0) + step);
      if ((job.downloaded ?? 0) >= (job.total ?? 100)) job.phase = 'installing';
    }
  }

  /** Completes the installing phase so finished jobs disappear from jobs(). */
  finish(id?: string) {
    for (const [jobID, job] of this.jobsData) {
      if (id && id !== jobID) continue;
      if (job.error) continue;
      job.done = true;
    }
  }
}

export const demoFixture = demo as FixtureData;
