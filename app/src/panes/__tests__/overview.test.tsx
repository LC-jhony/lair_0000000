import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import type { ReactNode } from 'react';
import { describe, expect, it, vi } from 'vitest';
import { FakeDaemon, demoFixture } from '../../api';
import { I18nProvider } from '../../i18n';
import type { LairController } from '../../useLair';
import { OverviewPane } from '../OverviewPane';

function renderPane(node: ReactNode) {
  render(<I18nProvider>{node}</I18nProvider>);
}

function controller(daemon: FakeDaemon): LairController {
  return {
    client: daemon as never,
    status: demoFixture.status,
    busy: '',
    error: '',
    refresh: vi.fn(async () => {}),
    clearError: vi.fn(),
    cancel: vi.fn(),
    act: async () => {},
  };
}

describe('carpetas de proyectos en General', () => {
  it('muestra las carpetas que informa el daemon', async () => {
    renderPane(<OverviewPane state={controller(new FakeDaemon())} />);
    expect(await screen.findByText('/home/dev/Lair')).toBeTruthy();
    expect(screen.queryByRole('button', { name: 'Migrate old folder' })).toBeNull();
  });

  it('añade una carpeta con un campo de texto', async () => {
    const daemon = new FakeDaemon();
    const add = vi.spyOn(daemon, 'projectDirsAdd');
    renderPane(<OverviewPane state={controller(daemon)} />);
    await screen.findByText('/home/dev/Lair');

    fireEvent.click(screen.getByRole('button', { name: 'Add folder' }));
    const dialog = await screen.findByRole('dialog', { name: 'Add project folder' });
    fireEvent.change(within(dialog).getByRole('textbox', { name: 'Folder path' }), {
      target: { value: '/srv/apps' },
    });
    fireEvent.click(within(dialog).getByRole('button', { name: 'Save' }));

    await waitFor(() => expect(add).toHaveBeenCalledWith('/srv/apps'));
    expect(await screen.findByText('/srv/apps')).toBeTruthy();
  });

  it('pide confirmación antes de quitar una carpeta', async () => {
    const daemon = new FakeDaemon();
    const remove = vi.spyOn(daemon, 'projectDirsRemove');
    renderPane(<OverviewPane state={controller(daemon)} />);
    await screen.findByText('/home/dev/Lair');

    fireEvent.click(screen.getByRole('button', { name: 'Remove folder /home/dev/Lair' }));
    const dialog = await screen.findByRole('dialog', { name: 'Remove project folder?' });
    expect(dialog.textContent).toContain('/home/dev/Lair');
    fireEvent.click(within(dialog).getByRole('button', { name: 'Remove folder' }));
    await waitFor(() => expect(remove).toHaveBeenCalledWith('/home/dev/Lair'));
    await waitFor(() => expect(screen.queryByText('/home/dev/Lair')).toBeNull());
  });

  it('importa los ajustes de Valet y muestra la salida', async () => {
    const daemon = new FakeDaemon();
    const importValet = vi.spyOn(daemon, 'projectImportValet');
    renderPane(<OverviewPane state={controller(daemon)} />);
    await screen.findByText('/home/dev/Lair');

    fireEvent.click(screen.getByRole('button', { name: 'Import Valet settings' }));
    await waitFor(() => expect(importValet).toHaveBeenCalled());
    expect((await screen.findByRole('status')).textContent).toContain('Imported Valet settings');
    expect(await screen.findByText('/home/dev/.valet/Sites')).toBeTruthy();
  });

  it('migra la carpeta antigua en minúsculas a la nueva', async () => {
    const daemon = new FakeDaemon({
      fixture: { ...demoFixture, project: { dirs: ['/home/dev/lair'] } },
    });
    const migrate = vi.spyOn(daemon, 'projectMigrateDirs');
    renderPane(<OverviewPane state={controller(daemon)} />);
    expect(await screen.findByText('/home/dev/lair')).toBeTruthy();

    fireEvent.click(screen.getByRole('button', { name: 'Migrate old folder' }));
    const dialog = await screen.findByRole('dialog', { name: 'Migrate old project folder?' });
    expect(dialog.textContent).toContain('/home/dev/lair');
    expect(dialog.textContent).toContain('/home/dev/Lair');
    fireEvent.click(within(dialog).getByRole('button', { name: 'Migrate old folder' }));

    await waitFor(() => expect(migrate).toHaveBeenCalledWith('/home/dev/lair', '/home/dev/Lair'));
    expect(await screen.findByText('/home/dev/Lair')).toBeTruthy();
  });
});
