import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import { FakeDaemon, demoFixture, type LairStatus } from '../../api';
import { I18nProvider } from '../../i18n';
import type { LairController } from '../../useLair';
import { LairConfigDialog } from '../LairConfigDialog';

function valetController(web: ReturnType<typeof vi.fn>, webView: LairStatus['web'] = demoFixture.status.web) {
  const daemon = new FakeDaemon({ fixture: demoFixture });
  const client = Object.assign(daemon, { web });
  const state: LairController = {
    client: client as never,
    status: { ...demoFixture.status, web: webView },
    busy: '',
    error: '',
    refresh: vi.fn(async () => {}),
    clearError: vi.fn(),
    cancel: vi.fn(),
    act: async () => {},
  };
  return state;
}

function renderDialog(web: ReturnType<typeof vi.fn>, webView?: LairStatus['web']) {
  return render(
    <I18nProvider>
      <LairConfigDialog state={valetController(web, webView)} open onClose={() => {}} />
    </I18nProvider>,
  );
}

function richController() {
  const daemon = new FakeDaemon({ fixture: demoFixture });
  const web = vi.fn(async () => ({ output: '', web: demoFixture.status.web }));
  const usePHP = vi.fn(async () => {});
  const useNode = vi.fn(async () => {});
  const projectDirs = vi.fn(async () => ({ dirs: ['/home/jhony/Lair'] }));
  const projectImportValet = vi.fn(async () => ({ output: 'importado' }));
  const client = Object.assign(daemon, { web, usePHP, useNode, projectDirs, projectImportValet });
  const state: LairController = {
    client: client as never,
    status: {
      ...demoFixture.status,
      home: '/home/jhony/.local/share/lair',
      php: [{ version: '8.4', minor: '8.4' }],
      node: ['v22.16.0'],
      defaults: { php: '8.4', node: 'v22.16.0' },
      dns: { host: 'lair.test', ok: true, resolved: ['127.0.0.1'] },
      ca: { exists: true, system_trusted: true, browser_trusted: true, site_certs: 1 },
    },
    busy: '',
    error: '',
    refresh: vi.fn(async () => {}),
    clearError: vi.fn(),
    cancel: vi.fn(),
    act: async (_label, fn) => { await fn(client as never); },
  };
  return { state, web, usePHP, useNode, projectDirs, projectImportValet };
}

describe('configuración de Lair', () => {
  it('muestra puertos, dominio y estado sin rastro de nginx', () => {
    const web = vi.fn(async () => ({ output: '', web: demoFixture.status.web }));
    renderDialog(web);
    expect(screen.getByText('Lair configuration')).toBeTruthy();
    expect(screen.getByText('Server status')).toBeTruthy();
    expect(screen.queryByText(/nginx/i)).toBeNull();
    expect((screen.getByLabelText('HTTP port') as HTMLInputElement).value).toBe('8080');
    expect((screen.getByLabelText('HTTPS port') as HTMLInputElement).value).toBe('8443');
    expect((screen.getByLabelText('Local domain') as HTMLInputElement).value).toBe('test');
  });

  it('permite desactivar HTTPS y aplica la configuración', async () => {
    const web = vi.fn(async (_op: string, _payload?: unknown) => ({ output: '', web: { ...demoFixture.status.web, https: false } }));
    renderDialog(web);
    const toggle = screen.getByRole('switch', { name: 'Enable HTTPS' });
    expect(toggle.getAttribute('aria-checked')).toBe('true');
    fireEvent.click(toggle);
    fireEvent.click(screen.getByRole('button', { name: 'Apply settings' }));
    await waitFor(() => expect(web).toHaveBeenCalled());
    const [op, payload] = web.mock.calls.at(-1)!;
    expect(op).toBe('configure');
    expect(payload).toMatchObject({
      https: false,
      http_port: 8080,
      https_port: 8443,
      bind: '127.0.0.1',
      domain: 'test',
    });
    expect(payload).not.toHaveProperty('engine');
  });

  it('rechaza puertos iguales antes de llamar al daemon', () => {
    const web = vi.fn(async () => ({ output: '', web: demoFixture.status.web }));
    renderDialog(web);
    fireEvent.change(screen.getByLabelText('HTTPS port'), { target: { value: '8080' } });
    fireEvent.click(screen.getByRole('button', { name: 'Apply settings' }));
    expect(web).not.toHaveBeenCalled();
    expect(screen.getByRole('alert').textContent).toContain('Ports must be different');
  });

  it('muestra 80 y 443 como puertos por defecto cuando el daemon no los informa', () => {
    const web = vi.fn(async () => ({ output: '', web: {} }));
    renderDialog(web, { https: true, domain: 'test' });
    expect((screen.getByLabelText('HTTP port') as HTMLInputElement).value).toBe('80');
    expect((screen.getByLabelText('HTTPS port') as HTMLInputElement).value).toBe('443');
  });

  it('ofrece cambiar a puertos sin privilegios cuando falta CAP_NET_BIND_SERVICE', async () => {
    const web = vi.fn(async () => ({ output: '', web: {} }));
    renderDialog(web, {
      ...demoFixture.status.web,
      http_port: 80,
      https_port: 443,
      error: 'listen tcp :80: bind: permission denied (CAP_NET_BIND_SERVICE)',
    });
    expect(screen.getByText('Ports below 1024 need extra privileges')).toBeTruthy();
    fireEvent.click(screen.getByRole('button', { name: 'Use unprivileged ports' }));
    await waitFor(() => expect(web).toHaveBeenCalled());
    expect(web.mock.calls.at(-1)).toEqual(['configure', expect.objectContaining({
      http_port: 8080,
      https_port: 8443,
      bind: '127.0.0.1',
      domain: 'test',
      https: true,
    })]);
  });

  it('muestra PHP, Node, carpetas y el estado de DNS y CA', async () => {
    const { state, projectDirs, projectImportValet } = richController();
    render(
      <I18nProvider>
        <LairConfigDialog state={state} open onClose={() => {}} />
      </I18nProvider>,
    );
    await waitFor(() => expect(projectDirs).toHaveBeenCalled());
    expect(screen.getByText('Default PHP')).toBeTruthy();
    expect(screen.getByText('Default Node.js')).toBeTruthy();
    expect(screen.getByText('Project folders (~/Lair)')).toBeTruthy();
    expect(screen.getByText('/home/jhony/Lair')).toBeTruthy();
    expect(screen.getByText('resolves')).toBeTruthy();
    expect(screen.getByText('installed')).toBeTruthy();
    expect(screen.getByText('system trust')).toBeTruthy();
    expect(screen.getByText('browser trust')).toBeTruthy();
    expect(screen.getByText('1 site certificates')).toBeTruthy();
    expect(screen.getByText(/setup-privileged\.sh/)).toBeTruthy();

    fireEvent.click(screen.getByRole('button', { name: 'Import Valet settings' }));
    await waitFor(() => expect(projectImportValet).toHaveBeenCalled());
  });
});
