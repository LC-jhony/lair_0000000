import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import type { ReactNode } from 'react';
import { describe, expect, it, vi } from 'vitest';
import { FakeDaemon, demoFixture } from '../../api';
import { I18nProvider } from '../../i18n';
import type { LairController } from '../../useLair';
import { ProjectDialog } from '../ProjectDialog';

function renderPane(node: ReactNode) {
  render(<I18nProvider>{node}</I18nProvider>);
}

function controller(client: FakeDaemon): LairController {
  return {
    client: client as never,
    status: demoFixture.status,
    busy: '',
    error: '',
    refresh: vi.fn(async () => {}),
    clearError: vi.fn(),
    cancel: vi.fn(),
    act: vi.fn(async () => {}),
  };
}

describe('asistente de nuevo proyecto', () => {
  it('valida el nombre y crea un proyecto SQLite', async () => {
    const daemon = new FakeDaemon({ fixture: demoFixture });
    const projectCreate = vi.spyOn(daemon, 'projectCreate');
    renderPane(<ProjectDialog open onClose={() => {}} state={controller(daemon)} />);

    expect(await screen.findByText('Laravel Installer 5.9.0')).toBeTruthy();
    expect(await screen.findByRole('button', { name: '1. Project' })).toBeTruthy();

    fireEvent.click(screen.getByRole('button', { name: 'Next' }));
    expect(screen.getByRole('alert').textContent).toContain('lowercase letters');

    fireEvent.change(screen.getByLabelText('Project name'), { target: { value: 'blog' } });
    fireEvent.click(screen.getByRole('button', { name: 'Next' }));
    expect(screen.queryByRole('alert')).toBeNull();

    fireEvent.click(screen.getByRole('button', { name: 'Next' }));
    fireEvent.click(screen.getByRole('button', { name: 'Next' }));
    // The wizard now follows the installed/configured engine (MariaDB in the
    // daemon), so pick SQLite explicitly for this case.
    fireEvent.click(screen.getByRole('button', { name: 'Database' }));
    fireEvent.click(await screen.findByRole('option', { name: 'sqlite' }));
    fireEvent.click(screen.getByRole('button', { name: 'Create project' }));

    await waitFor(() => expect(projectCreate).toHaveBeenCalled());
    expect(projectCreate.mock.calls[0]![0]).toMatchObject({
      name: 'blog',
      parent: '/home/dev/Lair',
      database: 'sqlite',
      secure: true,
      stack: 'blade',
    });
    expect(await screen.findByText(/blog.test/)).toBeTruthy();
  });

  it('pide credenciales cuando la base de datos no es SQLite', async () => {
    const daemon = new FakeDaemon({ fixture: demoFixture });
    const projectCreate = vi.spyOn(daemon, 'projectCreate');
    renderPane(<ProjectDialog open onClose={() => {}} state={controller(daemon)} />);
    fireEvent.change(screen.getByLabelText('Project name'), { target: { value: 'shop' } });
    fireEvent.click(screen.getByRole('button', { name: 'Next' }));
    fireEvent.click(screen.getByRole('button', { name: 'Next' }));
    fireEvent.click(screen.getByRole('button', { name: 'Next' }));

    fireEvent.click(screen.getByRole('button', { name: 'Database' }));
    fireEvent.click(await screen.findByRole('option', { name: 'mariadb' }));

    fireEvent.click(screen.getByRole('button', { name: 'Create project' }));
    expect(screen.getByRole('alert').textContent).toContain('valid database name');

    fireEvent.change(screen.getByLabelText('Database name'), { target: { value: 'shop_dev' } });
    fireEvent.change(screen.getByLabelText('Database user'), { target: { value: 'dev' } });
    fireEvent.change(screen.getByLabelText('Database password'), { target: { value: 'secret' } });
    fireEvent.click(screen.getByRole('button', { name: 'Create project' }));

    await waitFor(() => expect(projectCreate).toHaveBeenCalled());
    expect(projectCreate.mock.calls.at(-1)![0]).toMatchObject({
      database: 'mariadb', db_name: 'shop_dev', db_user: 'dev', db_password: 'secret',
    });
  });

  it('acepta contraseña vacía para el root local de MariaDB', async () => {
    const daemon = new FakeDaemon({ fixture: demoFixture });
    const projectCreate = vi.spyOn(daemon, 'projectCreate');
    renderPane(<ProjectDialog open onClose={() => {}} state={controller(daemon)} />);
    // Wait for projectInfo(): it selects the configured engine (MariaDB).
    await screen.findByText('Laravel Installer 5.9.0');
    fireEvent.change(screen.getByLabelText('Project name'), { target: { value: 'shop' } });
    fireEvent.click(screen.getByRole('button', { name: 'Next' }));
    fireEvent.click(screen.getByRole('button', { name: 'Next' }));
    fireEvent.click(screen.getByRole('button', { name: 'Next' }));

    fireEvent.change(screen.getByLabelText('Database name'), { target: { value: 'shop_dev' } });
    fireEvent.change(screen.getByLabelText('Database user'), { target: { value: 'root' } });
    fireEvent.click(screen.getByRole('button', { name: 'Create project' }));

    await waitFor(() => expect(projectCreate).toHaveBeenCalled());
    expect(projectCreate.mock.calls[0]![0]).toMatchObject({
      database: 'mariadb', db_name: 'shop_dev', db_user: 'root', db_password: '',
    });
  });

  it('muestra el fallo del job y permite reintentar sin perder la contraseña', async () => {
    const daemon = new FakeDaemon({ fixture: demoFixture });
    const projectCreate = vi.fn(async (_payload: unknown) => ({ id: 'project-blog' }));
    const job = vi.fn(async () => ({ id: 'project-blog', done: true, error: 'installer exploded' }));
    const log = vi.fn(async () => 'installer output');
    const client = Object.assign(daemon, { projectCreate, job, log });

    renderPane(<ProjectDialog open onClose={() => {}} state={controller(client)} />);
    fireEvent.change(screen.getByLabelText('Project name'), { target: { value: 'blog' } });
    fireEvent.click(screen.getByRole('button', { name: 'Next' }));
    fireEvent.click(screen.getByRole('button', { name: 'Next' }));
    fireEvent.click(screen.getByRole('button', { name: 'Next' }));
    fireEvent.click(screen.getByRole('button', { name: 'Database' }));
    fireEvent.click(await screen.findByRole('option', { name: 'mariadb' }));
    fireEvent.change(screen.getByLabelText('Database name'), { target: { value: 'blog_dev' } });
    fireEvent.change(screen.getByLabelText('Database user'), { target: { value: 'dev' } });
    fireEvent.change(screen.getByLabelText('Database password'), { target: { value: 'secret' } });
    fireEvent.click(screen.getByRole('button', { name: 'Create project' }));

    expect(await screen.findByText('Project creation failed.')).toBeTruthy();
    expect(screen.getByText('installer exploded')).toBeTruthy();
    expect(screen.getAllByRole('button', { name: 'Close' }).length).toBeGreaterThan(0);

    fireEvent.click(screen.getByRole('button', { name: 'Retry' }));
    await waitFor(() => expect(projectCreate).toHaveBeenCalledTimes(2));
    expect(projectCreate.mock.calls.at(-1)![0]).toMatchObject({ db_password: 'secret' });
  });
});
