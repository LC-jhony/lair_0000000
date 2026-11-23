import { fireEvent, render, screen } from '@testing-library/react';
import type { ReactNode } from 'react';
import { describe, expect, it, vi } from 'vitest';
import { FakeDaemon, demoFixture } from '../../api';
import { I18nProvider } from '../../i18n';
import type { LairController } from '../../useLair';
import { DatabasesPane } from '../DatabasesPane';
import { RuntimesPane } from '../RuntimesPane';

/** Wraps the fake daemon so individual calls can be asserted. */
function spyDaemon() {
  const daemon = new FakeDaemon({ fixture: demoFixture });
  const wrapped = Object.fromEntries(
    Object.getOwnPropertyNames(Object.getPrototypeOf(daemon))
      .filter((name) => name !== 'constructor')
      .map((name) => [name, (daemon as unknown as Record<string, (...args: unknown[]) => unknown>)[name]!.bind(daemon)]),
  ) as Record<string, (...args: never[]) => unknown>;
  wrapped.installPHP = vi.fn((version: string) => daemon.installPHP(version)) as never;
  wrapped.installNode = vi.fn((version: string) => daemon.installNode(version)) as never;
  wrapped.installService = vi.fn((id: string, version: string, port?: number) =>
    daemon.installService(id, version, port),
  ) as never;
  return wrapped;
}

function controller(client: ReturnType<typeof spyDaemon>): LairController {
  return {
    client: client as never,
    status: demoFixture.status,
    busy: '',
    error: '',
    refresh: vi.fn(async () => {}),
    clearError: vi.fn(),
    cancel: vi.fn(),
    act: async (_label, fn) => {
      await fn(client as never);
    },
  };
}

function renderPane(node: ReactNode) {
  render(<I18nProvider>{node}</I18nProvider>);
}

describe('selectores de instalación', () => {
  it('instala la versión concreta de PHP elegida', async () => {
    const client = spyDaemon();
    renderPane(<RuntimesPane state={controller(client)} />);
    const [phpSelect] = await screen.findAllByRole('button', { name: 'Version' });
    fireEvent.click(phpSelect!);
    fireEvent.click(await screen.findByRole('option', { name: '8.3.32' }));
    fireEvent.click(await screen.findByRole('button', { name: 'Install PHP 8.3.32' }));
    expect(client.installPHP).toHaveBeenCalledWith('8.3.32');
  });

  it('instala una versión de Node escrita a mano', async () => {
    const client = spyDaemon();
    renderPane(<RuntimesPane state={controller(client)} />);
    fireEvent.click(await screen.findByRole('button', { name: 'Other version' }));
    fireEvent.change(await screen.findByPlaceholderText('26, lts or 22.16.0'), { target: { value: '22.16.0' } });
    fireEvent.click(await screen.findByRole('button', { name: 'Install Node 22.16.0' }));
    expect(client.installNode).toHaveBeenCalledWith('22.16.0');
  });

  it('normaliza prefijos de Node antes de instalar', async () => {
    const client = spyDaemon();
    renderPane(<RuntimesPane state={controller(client)} />);
    fireEvent.click(await screen.findByRole('button', { name: 'Other version' }));
    fireEvent.change(await screen.findByPlaceholderText('26, lts or 22.16.0'), { target: { value: 'node 22.16.0' } });
    fireEvent.click(await screen.findByRole('button', { name: 'Install Node 22.16.0' }));
    expect(client.installNode).toHaveBeenCalledWith('22.16.0');
  });

  it('no muestra el error de versión de Node hasta que se sale del campo', async () => {
    const client = spyDaemon();
    renderPane(<RuntimesPane state={controller(client)} />);
    fireEvent.click(await screen.findByRole('button', { name: 'Other version' }));
    const input = await screen.findByPlaceholderText('26, lts or 22.16.0');
    fireEvent.change(input, { target: { value: 'abc' } });
    expect(screen.queryByRole('alert')).toBeNull();
    fireEvent.blur(input);
    expect(await screen.findByRole('alert')).toBeTruthy();
  });

  it('acepta la sintaxis de nvm (26, lts) sin forzar una versión exacta', async () => {
    const client = spyDaemon();
    renderPane(<RuntimesPane state={controller(client)} />);
    fireEvent.click(await screen.findByRole('button', { name: 'Other version' }));
    fireEvent.change(await screen.findByPlaceholderText('26, lts or 22.16.0'), { target: { value: '26' } });
    fireEvent.click(await screen.findByRole('button', { name: 'Install Node 26' }));
    expect(client.installNode).toHaveBeenCalledWith('26');
  });

  it('rellena la versión de Node desde los ejemplos', async () => {
    const client = spyDaemon();
    renderPane(<RuntimesPane state={controller(client)} />);
    fireEvent.click(await screen.findByRole('button', { name: 'Other version' }));
    fireEvent.click(await screen.findByRole('button', { name: 'lts' }));
    expect((await screen.findByPlaceholderText('26, lts or 22.16.0') as HTMLInputElement).value).toBe('lts');
    expect(screen.getByText('Ready to install')).toBeTruthy();
  });

  it('instala el motor y la versión concretos de base de datos', async () => {
    const client = spyDaemon();
    renderPane(<DatabasesPane state={controller(client)} />);
    fireEvent.click(await screen.findByRole('button', { name: 'Version' }));
    fireEvent.click(await screen.findByRole('option', { name: /PostgreSQL 18\.7\.0/ }));
    fireEvent.click(await screen.findByRole('button', { name: 'Install PostgreSQL 18.7.0' }));
    expect(client.installService).toHaveBeenCalledWith('postgres', '18.7.0', 5432);
  });
});
