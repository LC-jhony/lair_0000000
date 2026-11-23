import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import type { ReactNode } from 'react';
import { describe, expect, it, vi } from 'vitest';
import { FakeDaemon, demoFixture } from '../../api';
import { I18nProvider } from '../../i18n';
import type { LairController } from '../../useLair';
import { DebuggerPane } from '../DebuggerPane';
import { DumpsPane } from '../DumpsPane';
import { LogsPane } from '../LogsPane';
import { MailPane } from '../MailPane';
import { OverviewPane } from '../OverviewPane';
import { SystemPane } from '../SystemPane';
import type { PaneId } from '../types';

function controller() {
  const daemon = new FakeDaemon({ fixture: demoFixture });
  const state: LairController = {
    client: daemon,
    status: demoFixture.status,
    busy: '',
    error: '',
    refresh: vi.fn(async () => {}),
    clearError: vi.fn(),
    cancel: vi.fn(),
    act: async (_label, fn) => { await fn(daemon); },
  };
  return { state, daemon };
}

function renderPane(node: ReactNode) {
  return render(<I18nProvider>{node}</I18nProvider>);
}

describe('pane de correo', () => {
  it('muestra el SMTP, los mensajes y la configuración MAIL_*', async () => {
    const { state } = controller();
    renderPane(<MailPane state={state} />);

    expect(await screen.findByText('Welcome to your new site')).toBeInTheDocument();
    expect(screen.getByText(/MAIL_PORT=2525/)).toBeInTheDocument();
    expect(screen.getByText('Message detail')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Open Mail' })).toBeInTheDocument();
  });

  it('inicia y detiene el servidor SMTP', async () => {
    const { state, daemon } = controller();
    const mail = vi.spyOn(daemon, 'mail');
    renderPane(<MailPane state={state} />);
    await screen.findByText('Welcome to your new site');

    fireEvent.click(screen.getByRole('button', { name: 'Start' }));
    await waitFor(() => expect(screen.getByRole('button', { name: 'Stop' })).toBeInTheDocument());
    expect(mail).toHaveBeenCalledWith({ op: 'start' });

    fireEvent.click(screen.getByRole('button', { name: 'Stop' }));
    await waitFor(() => expect(screen.getByRole('button', { name: 'Start' })).toBeInTheDocument());
    expect(mail).toHaveBeenCalledWith({ op: 'stop' });
  });

  it('abre el detalle con pestañas y limpia la bandeja tras confirmar', async () => {
    const { state, daemon } = controller();
    renderPane(<MailPane state={state} />);
    await screen.findByText('Welcome to your new site');

    fireEvent.click(screen.getByRole('button', { name: /Welcome to your new site/ }));
    await waitFor(() => expect(screen.getByRole('tab', { name: 'Headers' })).toBeInTheDocument());

    fireEvent.click(screen.getByRole('button', { name: 'Clear all' }));
    const dialog = await screen.findByRole('dialog', { name: 'Clear the inbox?' });
    fireEvent.click(within(dialog).getByRole('button', { name: 'Clear' }));
    await waitFor(async () => {
      const list = await daemon.mail({ op: 'list' });
      expect(list.messages).toHaveLength(0);
    });
    expect(await screen.findByText('No messages yet')).toBeInTheDocument();
  });
});

describe('pane de dumps', () => {
  it('muestra la línea de tiempo y filtra por búsqueda', async () => {
    const { state } = controller();
    renderPane(<DumpsPane state={state} />);

    expect(await screen.findByText('select * from users where id = 1 limit 1')).toBeInTheDocument();
    const search = screen.getByRole('searchbox', { name: 'Search dumps' });
    fireEvent.change(search, { target: { value: 'login' } });

    expect(screen.getByText('POST /login')).toBeInTheDocument();
    expect(screen.queryByText('select * from users where id = 1 limit 1')).not.toBeInTheDocument();
  });

  it('activa la captura y respeta Latest Request Only', async () => {
    const { state, daemon } = controller();
    const dumps = vi.spyOn(daemon, 'dumps');
    renderPane(<DumpsPane state={state} />);
    await screen.findByText('select * from users where id = 1 limit 1');

    const capture = screen.getByRole('switch', { name: /Capture dumps & queries/ });
    expect(capture).toHaveAttribute('aria-checked', 'false');
    fireEvent.click(capture);
    await waitFor(() => expect(screen.getByRole('switch', { name: /Capture dumps & queries/ })).toHaveAttribute('aria-checked', 'true'));
    expect(dumps).toHaveBeenCalledWith({ op: 'capture-on' });

    const latest = screen.getByRole('switch', { name: /Latest Request Only/ });
    fireEvent.click(latest);
    expect(latest).toHaveAttribute('aria-checked', 'true');
    // The older /dashboard event is dropped; the newest of each request stays.
    expect(screen.queryByText('select * from users where id = 1 limit 1')).not.toBeInTheDocument();
    expect(screen.getByText('GET /api/health')).toBeInTheDocument();
  });

  it('pide confirmación antes de limpiar los eventos', async () => {
    const { state, daemon } = controller();
    renderPane(<DumpsPane state={state} />);
    await screen.findByText('select * from users where id = 1 limit 1');

    fireEvent.click(screen.getByRole('button', { name: 'Clear' }));
    const dialog = await screen.findByRole('dialog', { name: 'Clear captured dumps?' });
    fireEvent.click(within(dialog).getByRole('button', { name: 'Clear' }));

    await waitFor(async () => {
      const list = await daemon.dumps({ op: 'list' });
      expect(list.events).toHaveLength(0);
    });
    expect(await screen.findByText('No dumps captured')).toBeInTheDocument();
  });
});

describe('pane de debugger', () => {
  it('alterna la detección de Xdebug y de breakpoints', async () => {
    const { state, daemon } = controller();
    const debuggerCall = vi.spyOn(daemon, 'debugger');
    renderPane(<DebuggerPane state={state} />);

    const detect = await screen.findByRole('switch', { name: 'Detect Xdebug requests' });
    expect(detect).toHaveAttribute('aria-checked', 'false');
    fireEvent.click(detect);
    await waitFor(() => expect(screen.getByRole('switch', { name: 'Detect Xdebug requests' })).toHaveAttribute('aria-checked', 'true'));
    expect(debuggerCall).toHaveBeenCalledWith({ op: 'detect-on' });

    const breakpoints = screen.getByRole('switch', { name: 'Detect PhpStorm breakpoints' });
    fireEvent.click(breakpoints);
    await waitFor(() => expect(screen.getByRole('switch', { name: 'Detect PhpStorm breakpoints' })).toHaveAttribute('aria-checked', 'true'));
    expect(debuggerCall).toHaveBeenCalledWith({ op: 'breakpoints-on' });
  });

  it('muestra la guía de conexión y la disponibilidad por PHP', async () => {
    const { state } = controller();
    renderPane(<DebuggerPane state={state} />);

    expect(await screen.findByText(/XDEBUG_CONFIG/)).toBeInTheDocument();
    expect(screen.getByText(/XDEBUG_TRIGGER=1/)).toBeInTheDocument();
    expect(screen.getByText('PHP 8.4')).toBeInTheDocument();
    expect(screen.getByText('demo.test')).toBeInTheDocument();
  });
});

describe('pane de logs', () => {
  it('lista fuentes y muestra el contenido filtrado', async () => {
    const { state } = controller();
    renderPane(<LogsPane state={state} />);

    expect(await screen.findByRole('button', { name: /laravel.log/ })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /php-8.4/ })).toBeInTheDocument();
    await waitFor(() => expect(screen.getByText(/Site demo.test booted/)).toBeInTheDocument());
    expect(screen.getByRole('switch', { name: 'Auto-refresh' })).toHaveAttribute('aria-checked', 'false');
  });

  it('busca fuentes y limpia la vista tras confirmar', async () => {
    const { state } = controller();
    renderPane(<LogsPane state={state} />);
    await screen.findByRole('button', { name: /laravel.log/ });

    const search = screen.getByRole('searchbox', { name: 'Search logs' });
    fireEvent.change(search, { target: { value: 'mariadb' } });
    expect(screen.queryByRole('button', { name: /laravel.log/ })).not.toBeInTheDocument();
    expect(screen.getByRole('button', { name: /mariadb-11.4.7/ })).toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', { name: 'Clear' }));
    const dialog = await screen.findByRole('dialog', { name: 'Clear the log view?' });
    fireEvent.click(within(dialog).getByRole('button', { name: 'Clear' }));
    expect((await screen.findAllByText('No log selected')).length).toBeGreaterThan(0);
  });
});

describe('panes General y About', () => {
  it('muestra la configuración general y enlaza con Mail', () => {
    const { state } = controller();
    const onNavigate = vi.fn<(id: PaneId) => void>();
    renderPane(<OverviewPane state={state} onNavigate={onNavigate} />);

    expect(screen.getByText('*.test')).toBeInTheDocument();
    expect(screen.getByText('8080 / 8443')).toBeInTheDocument();
    expect(screen.getByText('/srv/demo')).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Open Mail' }));
    expect(onNavigate).toHaveBeenCalledWith('mail');
  });

  it('muestra versiones, enlaces y la hoja de desinstalación', async () => {
    const { state } = controller();
    renderPane(<SystemPane state={state} />);

    expect(screen.getByRole('link', { name: 'Docs' })).toHaveAttribute('href', expect.stringContaining('docs'));
    expect(screen.getByRole('link', { name: 'Report issue' })).toHaveAttribute('href', expect.stringContaining('issues'));

    fireEvent.click(screen.getByRole('button', { name: 'Uninstall' }));
    const dialog = await screen.findByRole('dialog', { name: 'Uninstall Lair' });
    expect(within(dialog).getByText(/lair uninstall --keep-data/)).toBeInTheDocument();

    const removeData = within(dialog).getByRole('switch', { name: 'Delete data as well' });
    fireEvent.click(removeData);
    expect(within(dialog).getByText(/lair uninstall --delete-data/)).toBeInTheDocument();
  });
});
