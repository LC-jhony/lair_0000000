import { afterEach, describe, expect, it, vi } from 'vitest';
import { ACTIONS, DaemonError, FakeDaemon, HttpDaemonClient, jobProgress, toComponentJob } from '../index';

function mockFetch(body: unknown, status = 200) {
  return vi.fn(async () => ({
    ok: status >= 200 && status < 300,
    status,
    json: async () => body,
  })) as unknown as typeof fetch;
}

describe('cliente HTTP del daemon', () => {
  it('envía acción y payload con el token en la cabecera', async () => {
    const fetchImpl = mockFetch({ ok: true, data: { versions: ['8.4.26'] } });
    const client = new HttpDaemonClient('http://127.0.0.1:9797/', 'token', fetchImpl);

    await expect(client.versions('php')).resolves.toEqual(['8.4.26']);
    const [url, init] = vi.mocked(fetchImpl).mock.calls[0];
    expect(url).toBe('http://127.0.0.1:9797/api');
    expect(init?.method).toBe('POST');
    expect((init?.headers as Record<string, string>)['X-Lair-Token']).toBe('token');
    expect(JSON.parse(String(init?.body))).toEqual({ action: 'versions', payload: { package: 'php' } });
  });

  it('convierte respuestas de error en DaemonError', async () => {
    const client = new HttpDaemonClient('http://127.0.0.1:9797', 'token', mockFetch({}, 401));
    await expect(client.status()).rejects.toMatchObject({ status: 401 });
    await expect(client.status()).rejects.toBeInstanceOf(DaemonError);
  });

  it('muestra el error del daemon en un 500 en vez del texto genérico', async () => {
    const client = new HttpDaemonClient(
      'http://127.0.0.1:9797',
      'token',
      mockFetch({ ok: false, error: 'fetch manifest.json from all base URLs: HTTP 404' }, 500),
    );
    const error = (await client.updateCatalog().catch((cause: unknown) => cause)) as DaemonError;
    expect(error).toBeInstanceOf(DaemonError);
    expect(error.status).toBe(500);
    expect(error.message).toContain('HTTP 404');
    expect(error.message).not.toContain('daemon responded');
  });

  it('tolera campos ausentes en las respuestas', async () => {
    const client = new HttpDaemonClient('http://127.0.0.1:9797', 'token', mockFetch({ ok: true }));
    await expect(client.versions('php')).resolves.toEqual([]);
    await expect(client.log('missing')).resolves.toBe('');
  });

  it('llama a fetch sin receiver (como exige el fetch nativo)', async () => {
    const strictFetch = (function (this: unknown) {
      if (this !== undefined) throw new Error('Illegal invocation');
      return Promise.resolve({ ok: true, status: 200, json: async () => ({ ok: true, data: {} }) });
    }) as unknown as typeof fetch;
    const client = new HttpDaemonClient('http://127.0.0.1:9797', 'token', strictFetch);
    await expect(client.call('status')).resolves.toEqual({});
  });

  it('rechaza un token vacío', () => {
    expect(() => new HttpDaemonClient('http://127.0.0.1:9797', '')).toThrow(DaemonError);
  });

  afterEach(() => vi.restoreAllMocks());

  it('asocia un timeout al fetch y propaga la cancelación del llamador', async () => {
    const caller = new AbortController();
    const timeout = new AbortController();
    vi.spyOn(AbortSignal, 'timeout').mockReturnValue(timeout.signal);
    let used: AbortSignal | undefined;
    const fetchImpl = vi.fn((_url: string, init?: RequestInit) => {
      used = init?.signal ?? undefined;
      return Promise.resolve({ ok: true, status: 200, json: async () => ({ ok: true }) });
    }) as unknown as typeof fetch;
    const client = new HttpDaemonClient('http://127.0.0.1:9797', 'token', fetchImpl);

    const pending = client.call('status', {}, caller.signal);
    expect(AbortSignal.timeout).toHaveBeenCalledWith(expect.any(Number));
    expect(used?.aborted).toBe(false);
    caller.abort();
    expect(used?.aborted).toBe(true);
    await expect(pending).resolves.toEqual({});
  });

  it('convierte un timeout en DaemonError con un mensaje sin detalles', async () => {
    const timeout = new AbortController();
    vi.spyOn(AbortSignal, 'timeout').mockReturnValue(timeout.signal);
    const fetchImpl = ((_url: string, init?: RequestInit) =>
      new Promise((_resolve, reject) => {
        init?.signal?.addEventListener('abort', () =>
          reject(new DOMException('fetch http://x/api?token=secreto', 'TimeoutError')),
        );
      })) as unknown as typeof fetch;
    const client = new HttpDaemonClient('http://127.0.0.1:9797', 'token', fetchImpl);

    const pending = client.status();
    timeout.abort();
    await expect(pending).rejects.toBeInstanceOf(DaemonError);
    await expect(pending).rejects.toMatchObject({ status: 0, message: 'request timed out' });
  });

  it('sanitiza errores de red y respuestas JSON inválidas', async () => {
    const brokenNetwork = vi.fn(async () => {
      throw new Error('fetch failed at http://x/api?token=secreto');
    }) as unknown as typeof fetch;
    const client = new HttpDaemonClient('http://127.0.0.1:9797', 'token', brokenNetwork);
    const networkFailure = (await client.status().catch((error: unknown) => error)) as DaemonError;
    expect(networkFailure).toBeInstanceOf(DaemonError);
    expect(networkFailure.message).toBe('network error');
    expect(networkFailure.message).not.toContain('secreto');

    const brokenJson = vi.fn(async () => ({
      ok: true,
      status: 200,
      json: async () => {
        throw new Error('token=secreto');
      },
    })) as unknown as typeof fetch;
    const other = new HttpDaemonClient('http://127.0.0.1:9797', 'token', brokenJson);
    await expect(other.status()).rejects.toMatchObject({
      status: 200,
      message: 'invalid response from daemon',
    });
  });
});

describe('progreso de jobs', () => {
  it('normaliza totales desconocidos y adapta el estado a la UI', () => {
    expect(jobProgress({ downloaded: 42, total: 100 })).toBeCloseTo(0.42);
    expect(jobProgress({ downloaded: 5, total: 0 })).toBeNull();
    expect(jobProgress({ downloaded: -1, total: 100 })).toBeNull();
    expect(jobProgress({})).toBeNull();

    expect(toComponentJob({ phase: 'downloading', downloaded: 42, total: 100 })).toMatchObject({
      state: 'running',
      phase: 'downloading',
      progress: 0.42,
    });
    expect(
      toComponentJob({ phase: 'installing', error: 'boom', done: true }, 'update'),
    ).toMatchObject({
      action: 'update',
      state: 'failed',
      phase: 'installing',
      error: 'boom',
    });
  });
});

describe('FakeDaemon', () => {
  it('sirve fixtures y simula un job hasta que desaparece', async () => {
    const fake = new FakeDaemon();
    const status = await fake.status();
    expect(status.home).toContain('lair');
    expect(status.services?.mariadb?.installed).toBe('11.4.7');

    const job = await fake.installService('mysql', '8.4.7', 3306);
    expect(job.phase).toBe('downloading');
    fake.advance();
    fake.advance();
    expect((await fake.job(job.id ?? '')).downloaded).toBe(50);
    fake.advance();
    fake.advance();
    expect((await fake.job(job.id ?? '')).phase).toBe('installing');
    await expect(fake.jobs()).resolves.toHaveLength(1);
    fake.finish();
    await expect(fake.jobs()).resolves.toHaveLength(0);
  });

  it('permite cancelar y forzar fallos para capturas', async () => {
    const fake = new FakeDaemon({ failOn: ['php/8.5.0-RC1'] });
    const failed = await fake.installPHP('8.5.0-RC1');
    fake.advance();
    expect((await fake.job(failed.id ?? '')).error).toContain('simulated');

    const canceled = await fake.installNode('v26.13.1');
    await fake.cancelJob(canceled.id ?? '');
    expect((await fake.job(canceled.id ?? '')).canceled).toBe(true);
  });

  it('despacha las mismas acciones que el daemon real', async () => {
    const fake = new FakeDaemon();
    await expect(fake.call(ACTIONS.status)).resolves.toMatchObject({ home: expect.any(String) });
    await expect(fake.call(ACTIONS.usePHP, { minor: '8.3' })).resolves.toBeUndefined();
    expect((await fake.status()).defaults?.php).toBe('8.3');
  });

  it('despacha mail, dumps y debugger con sus operaciones', async () => {
    const fake = new FakeDaemon();

    const status = await fake.call<{ port?: number; count?: number; running?: boolean }>(ACTIONS.mail, { op: 'status' });
    expect(status).toMatchObject({ port: 2525, count: 2, running: false });
    await expect(fake.call(ACTIONS.mail, { op: 'start' })).resolves.toMatchObject({ running: true });
    const found = await fake.call<{ message?: { subject?: string } }>(ACTIONS.mail, { op: 'message', id: 'msg-1001' });
    expect(found.message).toMatchObject({ subject: 'Welcome to your new site' });
    await expect(fake.call(ACTIONS.mail, { op: 'clear' })).resolves.toMatchObject({ count: 0 });

    await expect(fake.call(ACTIONS.dumps, { op: 'capture-on' })).resolves.toMatchObject({ capturing: true });
    const updated = await fake.call<{ options?: Record<string, boolean> }>(ACTIONS.dumps, { op: 'options', types: ['queries'], show_in_page: true });
    expect(updated.options).toMatchObject({ queries: true, logs: false, show_in_page: true });
    await expect(fake.call(ACTIONS.dumps, { op: 'clear' })).resolves.toMatchObject({ count: 0 });

    await expect(fake.call(ACTIONS.debugger, { op: 'detect-on' })).resolves.toMatchObject({ detect: true });
    await expect(fake.call(ACTIONS.debugger, { op: 'breakpoints-on' })).resolves.toMatchObject({ breakpoints: true });
    expect((await fake.status()).debug_running).toBe(true);
  });

  it('expone las fuentes de logs y los métodos de las nuevas acciones', async () => {
    const fake = new FakeDaemon();
    const sources = await fake.logSources();
    expect(sources).toEqual(expect.arrayContaining([
      expect.objectContaining({ kind: 'site', id: 'demo', files: expect.arrayContaining(['laravel.log']) }),
    ]));
    await expect(fake.call(ACTIONS.logsList)).resolves.toMatchObject({
      items: expect.arrayContaining(['php-8.4']),
      sources: expect.any(Array),
    });

    const fetchImpl = mockFetch({ ok: true, data: { running: true, port: 2525 } });
    const client = new HttpDaemonClient('http://127.0.0.1:9797', 'token', fetchImpl);
    await expect(client.mail({ op: 'status' })).resolves.toMatchObject({ running: true });
    expect(JSON.parse(String(vi.mocked(fetchImpl).mock.calls[0][1]?.body))).toEqual({
      action: 'mail',
      payload: { op: 'status' },
    });
  });

  it('expone la acción site con sus cinco operaciones', async () => {
    const fake = new FakeDaemon();
    const detail = await fake.site({ op: 'detail', name: 'demo.test' });
    expect(detail.detail).toMatchObject({ name: 'demo.test', framework: 'Laravel' });

    const env = await fake.site({ op: 'env', name: 'demo.test' });
    const secret = env.env?.find((entry) => entry.key === 'APP_KEY');
    expect(secret).toMatchObject({ secret: true });
    expect(secret?.value).toBeTruthy();

    const saved = await fake.site({ op: 'env-set', name: 'demo.test', key: 'APP_DEBUG', value: 'false' });
    expect(saved.env?.find((entry) => entry.key === 'APP_DEBUG')?.value).toBe('false');

    const domains = await fake.site({ op: 'domains', name: 'demo.test' });
    expect(domains.domains).toEqual(expect.arrayContaining(['demo.test']));

    const removed = await fake.site({ op: 'delete', name: 'demo.test', delete_files: true });
    expect(removed.output).toContain('demo.test');
    expect(fake.lastUnlink).toEqual({ name: 'demo.test', deleteFiles: true });
  });

  it('expone helpers para activar sitios y leer el vhost generado', async () => {
    const fetchImpl = mockFetch({ ok: true, data: { config: 'server_name demo.test;' } });
    const client = new HttpDaemonClient('http://127.0.0.1:9797', 'token', fetchImpl);
    await expect(client.siteSetEnabled('demo.test', false)).resolves.toMatchObject({
      config: 'server_name demo.test;',
    });
    expect(JSON.parse(String(vi.mocked(fetchImpl).mock.calls[0][1]?.body))).toEqual({
      action: 'site',
      payload: { op: 'set-enabled', name: 'demo.test', enabled: false },
    });
    await client.siteConfig('demo.test');
    expect(JSON.parse(String(vi.mocked(fetchImpl).mock.calls[1][1]?.body))).toEqual({
      action: 'site',
      payload: { op: 'config', name: 'demo.test' },
    });

    const fake = new FakeDaemon();
    expect((await fake.site({ op: 'detail', name: 'demo' })).detail?.enabled).toBe(true);
    const disabled = await fake.siteSetEnabled('demo', false);
    expect(disabled.detail?.enabled).toBe(false);
    expect((await fake.site({ op: 'detail', name: 'demo' })).detail?.enabled).toBe(false);
    expect((await fake.status()).sites?.find((site) => site.name === 'demo')?.enabled).toBe(false);

    // A disabled site still has a readable vhost.
    const off = await fake.siteConfig('demo');
    expect(off.config).toContain('server_name demo;');
    expect(off.config).toContain('root /srv/demo;');
    expect(off.config).toContain('disabled');
    await fake.siteSetEnabled('demo', true);
    expect((await fake.siteConfig('demo')).config).toContain('enabled');
  });

  it('expone la acción web solo con status, configure y los conmutadores HTTPS', async () => {
    const fake = new FakeDaemon();
    const status = await fake.web('status');
    expect(status.web).toMatchObject({
      https: true,
      http_port: 8080,
      https_port: 8443,
      bind: '127.0.0.1',
      domain: 'test',
    });
    for (const field of [
      'engine', 'mode', 'nginx_available', 'nginx_version', 'running',
      'config_path', 'detected_http_port', 'detected_https_port', 'port_warning',
    ]) {
      expect(status.web).not.toHaveProperty(field);
    }

    const configured = await fake.web('configure', { http_port: 9080, domain: 'lair.test' });
    expect(configured.web).toMatchObject({ http_port: 9080, domain: 'lair.test' });
    expect((await fake.status()).web).toMatchObject({ http_port: 9080, domain: 'lair.test' });

    expect((await fake.web('https-off')).web).toMatchObject({ https: false });
    expect((await fake.web('https-on')).web).toMatchObject({ https: true, redirect_http: true });
    await expect(fake.call(ACTIONS.web, { op: 'restart' })).rejects.toThrow(/unknown web operation/);
  });

  it('envía las operaciones web al daemon con su payload', async () => {
    const fetchImpl = mockFetch({ ok: true, data: { web: { https: true } } });
    const client = new HttpDaemonClient('http://127.0.0.1:9797', 'token', fetchImpl);
    await client.web('configure', {
      http_port: 9081,
      https: false,
      redirect_http: false,
      bind: '0.0.0.0',
      domain: 'lair.test',
    });
    expect(JSON.parse(String(vi.mocked(fetchImpl).mock.calls[0][1]?.body))).toEqual({
      action: 'web',
      payload: {
        op: 'configure',
        http_port: 9081,
        https: false,
        redirect_http: false,
        bind: '0.0.0.0',
        domain: 'lair.test',
      },
    });
  });

  it('envía delete_files en el payload de unlink', async () => {
    const fetchImpl = mockFetch({ ok: true, data: {} });
    const client = new HttpDaemonClient('http://127.0.0.1:9797', 'token', fetchImpl);
    await client.unlink('demo.test', true);
    expect(JSON.parse(String(vi.mocked(fetchImpl).mock.calls[0][1]?.body))).toEqual({
      action: 'unlink',
      payload: { name: 'demo.test', delete_files: true },
    });
    await client.unlink('demo.test');
    expect(JSON.parse(String(vi.mocked(fetchImpl).mock.calls[1][1]?.body))).toEqual({
      action: 'unlink',
      payload: { name: 'demo.test', delete_files: false },
    });
  });

  it('envía las nuevas ops de carpetas de proyectos al daemon', async () => {
    const fetchImpl = mockFetch({ ok: true, data: { dirs: [], output: 'ok' } });
    const client = new HttpDaemonClient('http://127.0.0.1:9797', 'token', fetchImpl);
    await client.projectDirs();
    await client.projectDirsAdd('/srv/apps');
    await client.projectDirsRemove('/srv/apps');
    await client.projectMigrateDirs('/home/dev/lair', '/home/dev/Lair');
    await client.projectImportValet();

    expect(vi.mocked(fetchImpl).mock.calls.map(([, init]) => JSON.parse(String(init?.body)))).toEqual([
      { action: 'project', payload: { op: 'dirs' } },
      { action: 'project', payload: { op: 'dirs-add', dir: '/srv/apps' } },
      { action: 'project', payload: { op: 'dirs-remove', dir: '/srv/apps' } },
      { action: 'project', payload: { op: 'migrate-dirs', from: '/home/dev/lair', to: '/home/dev/Lair' } },
      { action: 'project', payload: { op: 'import-valet' } },
    ]);
  });

  it('mantiene el estado mutable de las carpetas de proyectos en el FakeDaemon', async () => {
    const fake = new FakeDaemon();
    expect((await fake.projectDirs()).dirs).toEqual(['/home/dev/Lair']);
    expect((await fake.projectInfo()).projects_dir).toBe('/home/dev/Lair');

    const added = await fake.projectDirsAdd('/srv/apps');
    expect(added.dirs).toEqual(['/home/dev/Lair', '/srv/apps']);
    expect(added.output).toContain('/srv/apps');
    // Adding the same folder twice does not duplicate it.
    expect((await fake.projectDirsAdd('/srv/apps')).dirs).toEqual(['/home/dev/Lair', '/srv/apps']);

    const removed = await fake.projectDirsRemove('/srv/apps');
    expect(removed.dirs).toEqual(['/home/dev/Lair']);

    await fake.projectDirsAdd('/home/dev/lair');
    const migrated = await fake.projectMigrateDirs('/home/dev/lair', '/home/dev/Lair');
    expect(migrated.dirs).toEqual(['/home/dev/Lair']);
    expect(migrated.output).toContain('migrated');
    expect((await fake.call<{ dirs?: string[] }>(ACTIONS.project, { op: 'dirs' })).dirs).toEqual(['/home/dev/Lair']);

    const imported = await fake.projectImportValet();
    expect(imported.output).toContain('Valet');
    expect(imported.dirs).toContain('/home/dev/.valet/Sites');
  });

  it('cubre la paridad de acciones con la TUI', async () => {
    const fake = new FakeDaemon();
    const parked = await fake.call<{ text: string }>(ACTIONS.park, { path: '/srv/acme' });
    expect(parked.text).toBe('acme.test');
    await fake.call(ACTIONS.link, { name: 'blog.test', path: '/srv/blog' });
    await fake.call(ACTIONS.secure, { name: 'blog.test' });
    const sites = (await fake.status()).sites ?? [];
    expect(sites.find((site) => site.name === 'blog.test')?.secure).toBe(true);
    await fake.call(ACTIONS.unlink, { name: 'blog.test' });
    expect(((await fake.status()).sites ?? []).some((site) => site.name === 'blog.test')).toBe(false);

    await expect(fake.call<{ text: string }>(ACTIONS.createDB, { id: 'mariadb', name: 'acme_dev' })).resolves.toMatchObject({
      text: expect.stringContaining('acme_dev'),
    });
    await expect(fake.logsList()).resolves.toEqual(expect.arrayContaining(['php-8.4']));
    await expect(fake.call(ACTIONS.ensureShims)).resolves.toBeUndefined();
    await expect(fake.call(ACTIONS.restart)).resolves.toBeUndefined();
  });
});
