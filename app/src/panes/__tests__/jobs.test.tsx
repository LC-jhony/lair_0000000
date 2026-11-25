import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import { FakeDaemon, demoFixture } from '../../api';
import type { JobStatus } from '../../api';
import { I18nProvider } from '../../i18n';
import type { LairController } from '../../useLair';
import { JobsPane } from '../JobsPane';

function controller(client: unknown): LairController {
  return {
    client: client as never,
    status: demoFixture.status,
    busy: '',
    error: '',
    refresh: vi.fn(async () => {}),
    clearError: vi.fn(),
    cancel: vi.fn(),
    act: async (_label, fn) => { await fn(client as never); },
  };
}

describe('pane de trabajos', () => {
  it('lista jobs con estado, progreso y registro', async () => {
    const jobs: JobStatus[] = [
      { id: 'project-blog', package: 'project', version: 'blog', phase: 'installing', done: false },
      { id: 'mariadb-11.8.9', package: 'mariadb', version: '11.8.9', done: true },
      { id: 'php-8.4', package: 'php', version: '8.4', error: 'exit status 1' },
    ];
    const daemon = new FakeDaemon({ fixture: demoFixture });
    const client = Object.assign(daemon, {
      jobs: vi.fn(async () => jobs),
      log: vi.fn(async (name: string) => `registro de ${name}`),
    });

    render(<I18nProvider><JobsPane state={controller(client)} /></I18nProvider>);

    expect(await screen.findByText('project-blog')).toBeTruthy();
    expect(screen.getByText('mariadb-11.8.9')).toBeTruthy();
    expect(screen.getByText('exit status 1')).toBeTruthy();
    expect(screen.getAllByRole('progressbar').length).toBe(3);

    fireEvent.click(screen.getAllByRole('button', { name: 'View log' })[0]);
    await waitFor(() => expect(screen.getByText('registro de project-blog-install.log')).toBeTruthy());
  });
});
