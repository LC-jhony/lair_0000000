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

/** Mirrors a Linux daemon: the real status already includes sqlite3, and
 * MySQL has no signed build so it is not offered as a service. */
function linuxFixture() {
  const fixture = structuredClone(demoFixture);
  const services = fixture.status.services;
  if (!services) throw new Error('demo fixture has no services');
  services.mariadb = { ...services.mariadb, available: true, platform: 'linux-x64' };
  services.postgres = { ...services.postgres, available: true, platform: 'linux-x64' };
  services.redis = { state: 'missing', running: false, available: true, platform: 'linux-x64' };
  services.valkey = { state: 'missing', running: false, available: true, platform: 'linux-x64' };
  services.meilisearch = { state: 'missing', running: false, available: false, platform: 'linux-x64', unavailable_reason: 'no-build' };
  services.sqlite3 = { state: 'missing', running: false };
  delete services.mysql;
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

    // The daemon already reports sqlite3: it must appear only once, and MySQL
    // must not be offered because it has no signed build.
    expect(await screen.findByText('3 of 8 services installed')).toBeTruthy();
    expect(screen.getAllByText('SQLite')).toHaveLength(1);
    expect(screen.queryAllByText('mysql')).toHaveLength(0);
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
    services.meilisearch = {
      state: 'installed',
      running: false,
      port: 7700,
      installed: '1.30.0',
      version: '1.30.0',
      available: false,
      platform: 'linux-x64',
    };
    const daemon = new FakeDaemon({ fixture });

    render(
      <I18nProvider>
        <ServicesPane state={controller(daemon, fixture.status)} />
      </I18nProvider>,
    );

    expect((await screen.findAllByText('meilisearch')).length).toBeGreaterThan(0);
    expect(screen.getAllByText('No signed build in catalog').length).toBeGreaterThan(0);
    expect(screen.queryAllByText('mysql')).toHaveLength(0);
  });

  it('sugiere el siguiente puerto libre al instalar valkey con redis en 6379', async () => {
    const fixture = linuxFixture();
    const services = fixture.status.services;
    if (!services) throw new Error('demo fixture has no services');
    services.redis = {
      state: 'installed', running: true, port: 6379,
      installed: '7.4.6', version: '7.4.6', available: true, platform: 'linux-x64',
    };
    services.valkey = {
      state: 'missing', running: false, port: 6379, available: true, platform: 'linux-x64',
    };
    const daemon = new FakeDaemon({ fixture });
    const installService = vi.spyOn(daemon, 'installService');

    render(
      <I18nProvider>
        <ServicesPane state={controller(daemon, fixture.status)} />
      </I18nProvider>,
    );

    fireEvent.click(await screen.findByRole('button', { name: 'Install valkey' }));
    const confirm = await screen.findByRole('button', { name: 'Install valkey 9.2.0' });
    expect(screen.getByRole('spinbutton')).toHaveValue(6380);
    expect(screen.getByText('Port 6379 is taken; suggesting 6380')).toBeTruthy();
    fireEvent.click(confirm);
    await vi.waitFor(() => expect(installService).toHaveBeenCalledWith('valkey', '9.2.0', 6380));
  });
});
