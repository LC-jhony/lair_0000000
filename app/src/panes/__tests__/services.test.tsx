import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import { FakeDaemon, demoFixture } from '../../api';
import { I18nProvider } from '../../i18n';
import type { LairController } from '../../useLair';
import { ServicesPane } from '../ServicesPane';

function controller(client: unknown, status = demoFixture.status): LairController {
  return {
    client: client as never,
    status,
    busy: '',
    error: '',
    refresh: vi.fn(async () => {}),
    clearError: vi.fn(),
    cancel: vi.fn(),
    act: async (_label, fn) => { await fn(client as never); },
  };
}

/** Mirrors a Linux daemon: Valkey/Redis have builds, MySQL does not. */
function linuxFixture() {
  const fixture = structuredClone(demoFixture);
  const services = fixture.status.services;
  if (!services) throw new Error('demo fixture has no services');
  services.mariadb = { ...services.mariadb, available: true, platform: 'linux-x64' };
  services.postgres = { ...services.postgres, available: true, platform: 'linux-x64' };
  services.redis = { state: 'missing', running: false, available: true, platform: 'linux-x64' };
  services.valkey = { state: 'missing', running: false, available: true, platform: 'linux-x64' };
  services.mysql = { state: 'missing', running: false, available: false, platform: 'linux-x64', unavailable_reason: 'no-build' };
  fixture.versions.redis = ['7.4.6'];
  fixture.versions.valkey = ['9.2.0'];
  fixture.versions.sqlite3 = ['3.54.1'];
  return fixture;
}

describe('pestaña de servicios', () => {
  it('muestra valkey y redis, explica servicios no disponibles e instala desde la tarjeta', async () => {
    const fixture = linuxFixture();
    const daemon = new FakeDaemon({ fixture });
    const original = daemon.installService.bind(daemon);
    const installService = vi.fn((id: string, version: string, port?: number) =>
      original(id, version, port));
    const client = Object.assign(daemon, { installService });

    render(
      <I18nProvider>
        <ServicesPane state={controller(client, fixture.status)} />
      </I18nProvider>,
    );

    // 7 services reported by the daemon plus the SQLite information card.
    expect(await screen.findByText('3 of 8 services installed')).toBeTruthy();
    expect(screen.getAllByText('valkey').length).toBeGreaterThan(0);
    expect(screen.getAllByText('redis').length).toBeGreaterThan(0);
    expect(screen.getByText('Not available on linux-x64')).toBeTruthy();
    expect(screen.getByText('Ready for projects')).toBeTruthy();
    expect(await screen.findByRole('button', { name: 'Install CLI sqlite3' })).toBeTruthy();

    const installPostgres = await screen.findByRole('button', { name: 'Install postgres' });
    await waitFor(() => expect(installPostgres).not.toBeDisabled());
    fireEvent.click(installPostgres);

    const confirm = await screen.findByRole('button', { name: 'Install postgres 18.7.0' });
    fireEvent.click(confirm);
    await vi.waitFor(() => expect(installService).toHaveBeenCalled());
    expect(installService.mock.calls[0]!.slice(0, 2)).toEqual(['postgres', '18.7.0']);
  });

  it('mantiene la tarjeta de un servicio instalado aunque no tenga build firmado', async () => {
    const fixture = linuxFixture();
    const services = fixture.status.services;
    if (!services) throw new Error('demo fixture has no services');
    services.mysql = {
      state: 'installed',
      running: false,
      port: 3306,
      installed: '8.4.13',
      version: '8.4.13',
      available: false,
      platform: 'linux-x64',
    };
    const daemon = new FakeDaemon({ fixture });

    render(
      <I18nProvider>
        <ServicesPane state={controller(daemon, fixture.status)} />
      </I18nProvider>,
    );

    expect((await screen.findAllByText('mysql')).length).toBeGreaterThan(0);
    expect(screen.getAllByText('No signed build in catalog').length).toBeGreaterThan(0);
  });
});
