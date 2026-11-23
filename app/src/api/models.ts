/** A job snapshot from the daemon (installer.Status on the wire). */
export interface JobStatus {
  id?: string;
  phase?: string;
  downloaded?: number;
  total?: number;
  error?: string;
  done?: boolean;
  canceled?: boolean;
}

/** Tolerant snapshot: new daemon fields must never break the app. */
export interface LairStatus {
  home?: string;
  php?: PhpInfo[];
  node?: string[];
  services?: Record<string, ServiceView>;
  sites?: SiteView[];
  web?: WebView;
  /** Read-only DNS probe for the development domain. */
  dns?: DNSStatus;
  /** Read-only local CA snapshot. */
  ca?: CAStatus;
  composer?: string;
  jobs?: JobStatus[];
  defaults?: Record<string, string>;
  /** Optional counters published by daemons that serve the tooling panes. */
  mail_count?: number;
  dump_count?: number;
  debug_running?: boolean;
}

export interface PhpInfo {
  version?: string;
  minor?: string;
  default?: boolean;
  [key: string]: unknown;
}

export type ServiceState = 'installed' | 'missing' | 'unknown' | string;

export interface ServiceView {
  id?: string;
  active?: string;
  installed?: string | string[];
  state?: ServiceState;
  running?: boolean;
  port?: number;
  version?: string;
  pid?: number;
  last_error?: string;
  /** False when the signed catalog has no build for the daemon's platform. */
  available?: boolean;
  platform?: string;
  unavailable_reason?: string;
  [key: string]: unknown;
}

export interface SiteView {
  name?: string;
  host?: string;
  root?: string;
  path?: string;
  url?: string;
  php?: string;
  node?: string;
  database?: string;
  php_socket?: string;
  secure?: boolean;
  /** A disabled site stays registered but its virtual host is not served. */
  enabled?: boolean;
  root_state?: string;
  domains?: string[];
  [key: string]: unknown;
}

/** Full inspection payload returned by the `site` daemon action. */
export interface SiteDetail {
  name?: string;
  path?: string;
  url?: string;
  type?: string;
  framework?: string;
  php?: string;
  php_version?: string;
  php_source?: string;
  php_file?: string;
  node?: string;
  node_source?: string;
  node_file?: string;
  database?: string;
  database_host?: string;
  database_port?: number | string;
  database_name?: string;
  secure?: boolean;
  /** Whether the site's virtual host is currently served by the web layer. */
  enabled?: boolean;
  root?: string;
  root_state?: string;
  root_suggestions?: string[];
  domains?: string[];
  problem?: string;
  notice?: string;
  [key: string]: unknown;
}

/** One `.env` line. Secrets are masked server-side and must never be rendered. */
export interface SiteEnvEntry {
  key?: string;
  value?: string;
  secret?: boolean;
  set?: boolean;
  [key: string]: unknown;
}

export interface SiteResult {
  detail?: SiteDetail;
  env?: SiteEnvEntry[];
  domains?: string[];
  /** Generated virtual host text returned by the `config` operation. */
  config?: string;
  output?: string;
  /** Browser URL returned by the `open` operation. */
  url?: string;
}

export type SiteOp = 'detail' | 'env' | 'env-set' | 'domains' | 'delete' | 'set-enabled' | 'config' | 'open';

export interface SitePayload {
  op: SiteOp;
  name?: string;
  key?: string;
  value?: string;
  domains?: string[];
  /** New state for the `set-enabled` operation. */
  enabled?: boolean;
  delete_files?: boolean;
}

export interface DNSStatus {
  host?: string;
  ok?: boolean;
  resolved?: string[];
  detail?: string;
}

export interface CAStatus {
  exists?: boolean;
  system_trusted?: boolean;
  browser_trusted?: boolean;
  site_certs?: number;
  path?: string;
}

/** Web layer snapshot returned by the `web` daemon action. */
export interface WebView {
  https?: boolean;
  http_port?: number;
  https_port?: number;
  bind?: string;
  domain?: string;
  redirect_http?: boolean;
  warning?: string;
  error?: string;
}

export interface WebResult {
  output?: string;
  web?: WebView;
}

export type WebOp = 'status' | 'configure' | 'https-on' | 'https-off';

export interface WebConfigPayload {
  http_port?: number;
  https_port?: number;
  https?: boolean;
  redirect_http?: boolean;
  bind?: string;
  domain?: string;
}

/** Capabilities reported by `project info` for the new-project wizard. */
/** Tolerant views of the tooling payloads: new daemon fields must never break the UI. */

export interface MailAttachment {
  name?: string;
  content_type?: string;
  size?: number;
  [key: string]: unknown;
}

export interface MailMessage {
  id?: string;
  from?: string;
  to?: string;
  subject?: string;
  date?: string;
  body?: string;
  /** Optional richer representations; older daemons only expose `body`. */
  html?: string;
  text?: string;
  raw?: string;
  headers?: string | Record<string, unknown>;
  /** Some daemons send plain file names instead of attachment objects. */
  attachments?: Array<MailAttachment | string>;
  [key: string]: unknown;
}

export interface MailResult {
  raw?: string;
  running?: boolean;
  port?: number;
  count?: number;
  env?: string;
  messages?: MailMessage[];
  message?: MailMessage | null;
  output?: string;
}

export type MailOp = 'status' | 'start' | 'stop' | 'list' | 'message' | 'clear';

export interface MailPayload {
  op: MailOp;
  id?: string;
  /** SMTP username selects the mailbox; empty means the default inbox. */
  inbox?: string;
}

export interface DumpsOptions {
  queries?: boolean;
  logs?: boolean;
  views?: boolean;
  jobs?: boolean;
  requests?: boolean;
  show_in_page?: boolean;
  [key: string]: unknown;
}

export interface DumpEvent {
  id?: string;
  site?: string;
  type?: string;
  time?: string;
  request?: string;
  message?: string;
  [key: string]: unknown;
}

export interface DumpsResult {
  capturing?: boolean;
  options?: DumpsOptions;
  count?: number;
  events?: DumpEvent[];
  output?: string;
}

export type DumpsOp =
  | 'status'
  | 'capture-on'
  | 'capture-off'
  | 'options'
  | 'list'
  | 'clear';

export interface DumpsPayload {
  op: DumpsOp;
  types?: string[];
  show_in_page?: boolean;
  site?: string;
}

export interface DebuggerVersion {
  version?: string;
  available?: boolean;
}

export interface DebuggerResult {
  detect?: boolean;
  breakpoints?: boolean;
  port?: number;
  mode?: string;
  versions?: DebuggerVersion[];
  domains?: string[];
  output?: string;
}

export type DebuggerOp = 'status' | 'detect-on' | 'detect-off' | 'breakpoints-on' | 'breakpoints-off';

export interface DebuggerPayload {
  op: DebuggerOp;
}

export interface LogSource {
  kind?: 'site' | 'service' | string;
  id?: string;
  name?: string;
  path?: string;
  files?: string[];
}

/** Shape of `logs-list`; older daemons only send `items`. */
export interface NameList {
  items?: string[];
  sources?: LogSource[];
}

export interface ProjectInfo {
  laravel_installed?: boolean;
  laravel_version?: string;
  installer_flags?: string[];
  composer?: string;
  php?: string[];
  node?: string[];
  pdo_drivers?: string[];
  node_managers?: string[];
  databases?: string[];
  projects_dir?: string;
  hint?: string;
}

export type ProjectOp =
  | 'info'
  | 'create'
  | 'set-db'
  | 'sync'
  | 'dirs'
  | 'dirs-add'
  | 'dirs-remove'
  | 'migrate-dirs'
  | 'import-valet';

export type ProjectStack = 'blade' | 'react' | 'vue' | 'svelte' | 'livewire';
export type ProjectTesting = 'pest' | 'phpunit';
export type ProjectNodeManager = 'npm' | 'pnpm' | 'bun' | 'yarn' | 'none';
export type ProjectDatabase = 'sqlite' | 'mysql' | 'mariadb' | 'pgsql' | 'sqlsrv';

export interface ProjectCreatePayload {
  name: string;
  parent: string;
  secure: boolean;
  stack: ProjectStack;
  auth: boolean;
  testing: ProjectTesting;
  boost: boolean;
  node_manager: ProjectNodeManager;
  git: boolean;
  database: ProjectDatabase;
  db_name?: string;
  db_user?: string;
  db_password?: string;
  db_host?: string;
  db_port?: number;
  create_db: boolean;
  start_service: boolean;
}

export interface ProjectResult extends JobStatus {
  site?: string;
  path?: string;
  info?: ProjectInfo;
  /** Folders the daemon scans for projects (`dirs` operation). */
  dirs?: string[];
  /** Human-readable result of `dirs-add`, `migrate-dirs`, `import-valet`, ... */
  output?: string;
}

export interface ComponentJobView {
  action: 'install' | 'update';
  state: 'running' | 'failed';
  phase: 'downloading' | 'installing';
  progress: number | null;
  error?: string;
  startedAt?: string;
}

/** -1, zero totals, NaN and out-of-range values mean "unknown progress". */
export function jobProgress(job: JobStatus): number | null {
  const total = job.total;
  const downloaded = job.downloaded;
  if (typeof total !== 'number' || typeof downloaded !== 'number') return null;
  if (!Number.isFinite(total) || !Number.isFinite(downloaded) || total <= 0) return null;
  const ratio = downloaded / total;
  return ratio >= 0 && ratio <= 1 ? ratio : null;
}

/** Adapts the daemon job to the UI contract used by the job components. */
export function toComponentJob(job: JobStatus, action: 'install' | 'update' = 'install'): ComponentJobView {
  const failed = Boolean(job.error) || Boolean(job.canceled);
  return {
    action,
    state: failed ? 'failed' : 'running',
    phase: job.phase === 'installing' ? 'installing' : 'downloading',
    progress: jobProgress(job),
    error: job.error || undefined,
  };
}

/** All daemon actions used by the desktop app. */
export interface DaemonClient {
  call<T>(action: string, payload?: unknown, signal?: AbortSignal): Promise<T>;
  status(signal?: AbortSignal): Promise<LairStatus>;
  versions(packageName: string): Promise<string[]>;
  installPHP(version: string): Promise<JobStatus>;
  installNode(version: string): Promise<JobStatus>;
  uninstallPHP(minor: string): Promise<void>;
  updateNode(major: string): Promise<JobStatus>;
  uninstallNode(major: string): Promise<void>;
  updateComposer(): Promise<{ text?: string }>;
  updateNpm(): Promise<{ text?: string }>;
  retryJob(id: string): Promise<JobStatus>;
  installService(id: string, version: string, port?: number): Promise<JobStatus>;
  job(id: string, signal?: AbortSignal): Promise<JobStatus>;
  jobs(): Promise<JobStatus[]>;
  cancelJob(id: string): Promise<{ canceled?: boolean }>;
  log(name: string): Promise<string>;
  logsList(): Promise<string[]>;
  /** Optional log sources (sites/services) reported by newer daemons. */
  logSources(): Promise<LogSource[]>;
  mail(payload: MailPayload): Promise<MailResult>;
  mailRaw(id: string, inbox?: string): Promise<string>;
  dumps(payload: DumpsPayload): Promise<DumpsResult>;
  debugger(payload: DebuggerPayload): Promise<DebuggerResult>;
  usePHP(minor: string): Promise<void>;
  useNode(version: string): Promise<void>;
  useService(id: string, version: string): Promise<void>;
  restartService(id: string): Promise<void>;
  stopService(id: string): Promise<void>;
  uninstallService(id: string, version: string, deleteData?: boolean): Promise<void>;
  ensureDatabase(): Promise<JobStatus & { already_installed?: boolean }>;
  createDB(id: string, name: string): Promise<string>;
  setDefaultDB(id: string): Promise<void>;
  park(path: string): Promise<string>;
  link(name: string, path: string): Promise<void>;
  /** Removes the site. `deleteFiles` also removes the project folder (default false). */
  unlink(name: string, deleteFiles?: boolean): Promise<void>;
  /** Inspects and edits one site: detail, env, env-set, domains, delete, set-enabled, config. */
  site(payload: SitePayload): Promise<SiteResult>;
  /** Enables or disables a site's virtual host without removing the site. */
  siteSetEnabled(name: string, enabled: boolean): Promise<SiteResult>;
  /** Reads the generated virtual host, including for disabled sites. */
  siteConfig(name: string): Promise<SiteResult>;
  siteOpen(name: string): Promise<SiteResult>;
  secure(name: string, on?: boolean): Promise<void>;
  web(op: WebOp, payload?: WebConfigPayload): Promise<WebResult>;
  projectInfo(): Promise<ProjectInfo>;
  projectCreate(payload: ProjectCreatePayload): Promise<ProjectResult>;
  projectSetDB(name: string, database: string): Promise<void>;
  projectSync(): Promise<void>;
  /** Reads and manages the folders scanned for projects. */
  projectDirs(): Promise<ProjectResult>;
  projectDirsAdd(dir: string): Promise<ProjectResult>;
  projectDirsRemove(dir: string): Promise<ProjectResult>;
  projectMigrateDirs(from: string, to: string): Promise<ProjectResult>;
  projectImportValet(): Promise<ProjectResult>;
  installComposer(): Promise<{ composer_path?: string; bin_dir?: string; warnings?: string[] }>;
  pin(tool: string, value: string, dir?: string): Promise<void>;
  isolate(tool: string, value: string, dir?: string): Promise<void>;
  ensureShims(signal?: AbortSignal): Promise<void>;
  restart(): Promise<void>;
  updateCatalog(): Promise<string>;
}

/** Installed versions can arrive as a string or a JSON array from the daemon. */
export function installedVersions(service: ServiceView): string[] {
  const value = service.installed;
  if (typeof value === 'string') return value ? [value] : [];
  if (Array.isArray(value)) return value.filter((v): v is string => typeof v === 'string');
  return [];
}

export function activeVersion(service: ServiceView): string {
  return service.active || installedVersions(service)[0] || service.version || '';
}
