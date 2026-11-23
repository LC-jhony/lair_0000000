import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import type { ReactNode } from 'react';
import { describe, expect, it, vi } from 'vitest';
import { FakeDaemon, demoFixture } from '../../api';
import { I18nProvider } from '../../i18n';
import type { LairController } from '../../useLair';
import type { PaneId } from '../types';
import { SiteDetailDialog } from '../SiteDetailDialog';
import { SitesPane } from '../SitesPane';

function renderPane(node: ReactNode) {
  render(<I18nProvider>{node}</I18nProvider>);
}

function controller(daemon: FakeDaemon): LairController {
  return {
    client: daemon as never,
    status: {
      ...demoFixture.status,
      web: { ...demoFixture.status.web, domain: 'test', http_port: 8080, https_port: 8443, https: true },
      sites: [
        {
          name: 'blog.test',
          root: '/home/jhony/lair/blog',
          path: '/home/jhony/lair/blog',
          php: '8.4',
          database: 'mariadb',
          secure: true,
        },
      ],
    },
    busy: '',
    error: '',
    refresh: vi.fn(async () => {}),
    cancel: vi.fn(),
    clearError: vi.fn(),
    act: async (_label, fn) => { await fn(daemon as never); },
  };
}

describe('pestaña Sites', () => {
  it('muestra el proyecto, el host virtual y la URL HTTPS', () => {
    renderPane(<SitesPane state={controller(new FakeDaemon({ fixture: demoFixture }))} />);
    expect(screen.getByRole('heading', { name: 'blog' })).toBeTruthy();
    const link = screen.getByRole('link', { name: /blog\.test/ }) as HTMLAnchorElement;
    expect(link.getAttribute('href')).toBe('https://blog.test:8443');
    expect(link.textContent?.trim()).toBe('blog.test');
    expect(screen.getByText('mariadb')).toBeTruthy();
  });

  it('permite desactivar HTTPS de un sitio', async () => {
    const daemon = new FakeDaemon({ fixture: demoFixture });
    const secure = vi.spyOn(daemon, 'secure');
    renderPane(<SitesPane state={controller(daemon)} />);
    fireEvent.click(await screen.findByRole('switch', { name: 'HTTPS' }));
    await waitFor(() => expect(secure).toHaveBeenCalledWith('blog.test', false));
  });

  it('no expone el diálogo de configuración redundante', async () => {
    const daemon = new FakeDaemon({ fixture: demoFixture });
    renderPane(<SitesPane state={controller(daemon)} />);
    expect(screen.queryByRole('button', { name: 'Configure site' })).toBeNull();
    expect(await screen.findByRole('button', { name: 'Details' })).toBeTruthy();
  });
});

describe('inspector avanzado de Sites', () => {
  const openDetails = async (daemon: FakeDaemon, onNavigate?: (id: PaneId) => void) => {
    renderPane(<SitesPane state={controller(daemon)} onNavigate={onNavigate} />);
    fireEvent.click(await screen.findByRole('button', { name: 'Details' }));
    await screen.findByRole('dialog', { name: 'Site: blog.test' });
  };

  it('muestra las siete pestañas con datos del FakeDaemon', async () => {
    await openDetails(new FakeDaemon({ fixture: demoFixture }));
    await waitFor(() => expect(screen.getByText('Laravel')).toBeTruthy());
    const tabs = screen.getAllByRole('tab');
    expect(tabs.map((tab) => tab.textContent)).toEqual([
      'Overview', 'Environment', 'Database', 'Mail', 'Queues', 'PHP & Node.js', 'Logs',
    ]);
    expect(screen.getByText(/blog\.test:(8080|8443)/)).toBeTruthy();
  });

  it('enmascara los secretos y guarda un valor con env-set', async () => {
    const daemon = new FakeDaemon({ fixture: demoFixture });
    const site = vi.spyOn(daemon, 'site');
    await openDetails(daemon);
    fireEvent.click(await screen.findByRole('tab', { name: 'Environment' }));
    await waitFor(() => expect(screen.getByRole('button', { name: 'Save value APP_DEBUG' })).toBeTruthy());

    // The secret value never reaches the DOM, not even as an input placeholder.
    expect(document.body.textContent).not.toContain('demo-secret-key-do-not-show');
    expect(screen.getAllByText(/••••••/).length).toBeGreaterThan(0);

    fireEvent.change(screen.getByRole('textbox', { name: 'New value APP_DEBUG' }), { target: { value: 'false' } });
    fireEvent.click(screen.getByRole('button', { name: 'Save value APP_DEBUG' }));
    await waitFor(() => expect(site).toHaveBeenCalledWith({ op: 'env-set', name: 'blog.test', key: 'APP_DEBUG', value: 'false' }));
  });

  it('navega a Mail y Logs desde las pestañas correspondientes', async () => {
    const onNavigate = vi.fn<(id: PaneId) => void>();
    await openDetails(new FakeDaemon({ fixture: demoFixture }), onNavigate);

    fireEvent.click(screen.getByRole('tab', { name: 'Mail' }));
    fireEvent.click(await screen.findByRole('button', { name: 'Open Mail pane' }));
    expect(onNavigate).toHaveBeenCalledWith('mail');

    fireEvent.click(screen.getByRole('tab', { name: 'Queues' }));
    fireEvent.click(await screen.findByRole('button', { name: 'Open Logs pane' }));
    expect(onNavigate).toHaveBeenCalledWith('logs');
  });

  it('abre el visor de logs filtrado al sitio', async () => {
    await openDetails(new FakeDaemon({ fixture: demoFixture }));
    fireEvent.click(screen.getByRole('tab', { name: 'Logs' }));
    await waitFor(() => expect(screen.getByText(/Site demo\.test booted/)).toBeTruthy());
    fireEvent.click(screen.getByRole('button', { name: 'Open viewer' }));
    expect(screen.getByRole('dialog', { name: /Site logs/ })).toBeTruthy();
  });

  it('confirma el borrado y solo elimina la carpeta cuando se marca el checkbox', async () => {
    const daemon = new FakeDaemon({ fixture: demoFixture });
    const unlink = vi.spyOn(daemon, 'unlink');
    renderPane(<SitesPane state={controller(daemon)} />);

    fireEvent.click(screen.getByRole('button', { name: 'Delete site blog.test' }));
    const checkbox = screen.getByRole('checkbox', { name: 'Delete project folder too' });
    expect((checkbox as HTMLInputElement).checked).toBe(false);
    fireEvent.click(screen.getByRole('button', { name: 'Delete site' }));
    await waitFor(() => expect(unlink).toHaveBeenCalledWith('blog.test', false));

    fireEvent.click(screen.getByRole('button', { name: 'Delete site blog.test' }));
    fireEvent.click(screen.getByRole('checkbox', { name: 'Delete project folder too' }));
    fireEvent.click(screen.getByRole('button', { name: 'Delete site' }));
    await waitFor(() => expect(unlink).toHaveBeenCalledWith('blog.test', true));
  });

  it('busca sitios por nombre y muestra contadores', async () => {
    const daemon = new FakeDaemon({ fixture: demoFixture });
    renderPane(<SitesPane state={controller(daemon)} />);
    expect(screen.getByText('1 sites')).toBeTruthy();
    expect(screen.getByText('1 with HTTPS')).toBeTruthy();

    fireEvent.change(screen.getByRole('searchbox', { name: 'Search sites' }), { target: { value: 'nope' } });
    expect(await screen.findByText('No sites match your search.')).toBeTruthy();
    expect(screen.queryByRole('heading', { name: 'blog' })).toBeNull();
  });

  it('activa y desactiva el host virtual desde el inspector', async () => {
    const daemon = new FakeDaemon({ fixture: demoFixture });
    const siteSetEnabled = vi.spyOn(daemon, 'siteSetEnabled');
    await openDetails(daemon);
    await waitFor(() => expect(screen.getByText('Laravel')).toBeTruthy());

    const toggle = () => screen.getByRole('switch', { name: 'Enabled' });
    expect(toggle().getAttribute('aria-checked')).toBe('true');
    fireEvent.click(toggle());
    await waitFor(() => expect(siteSetEnabled).toHaveBeenCalledWith('blog.test', false));
    await waitFor(() => expect(toggle().getAttribute('aria-checked')).toBe('false'));

    fireEvent.click(toggle());
    await waitFor(() => expect(siteSetEnabled).toHaveBeenCalledWith('blog.test', true));
    await waitFor(() => expect(toggle().getAttribute('aria-checked')).toBe('true'));
  });

  it('muestra el vhost generado aunque el sitio esté desactivado', async () => {
    const daemon = new FakeDaemon({ fixture: demoFixture });
    await daemon.siteSetEnabled('blog.test', false);
    const siteConfig = vi.spyOn(daemon, 'siteConfig');
    await openDetails(daemon);
    await waitFor(() => expect(screen.getByText('Laravel')).toBeTruthy());

    fireEvent.click(screen.getByRole('button', { name: 'View generated vhost' }));
    const dialog = await screen.findByRole('dialog', { name: /Generated vhost/ });
    await waitFor(() => expect(siteConfig).toHaveBeenCalledWith('blog.test'));
    await waitFor(() => expect(within(dialog).getByText(/server_name blog\.test;/)).toBeTruthy());
    expect(within(dialog).getByText(/disabled/)).toBeTruthy();
  });

  it('avisa destacadamente cuando la carpeta del proyecto no está lista', async () => {
    const daemon = new FakeDaemon({ fixture: demoFixture });
    renderPane(
      <SiteDetailDialog
        open
        site={{ name: 'missing.test' }}
        state={controller(daemon)}
        onClose={() => {}}
      />,
    );
    expect(await screen.findByRole('dialog', { name: 'Site: missing.test' })).toBeTruthy();
    const warning = await screen.findByRole('alert');
    expect(warning.textContent).toContain('not ready (missing)');
    expect(warning.textContent).toContain('Expected project folder: /home/dev/lair/missing');
  });
});
