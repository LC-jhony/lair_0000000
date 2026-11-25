import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { useState, type ReactNode } from 'react';
import { describe, expect, it, vi } from 'vitest';
import { I18nProvider } from '../../i18n';
import {
  Badge,
  CheckForUpdatesLine,
  CodeBlock,
  ComponentJobFailure,
  ComponentJobProgress,
  DumpsTimeline,
  LogViewer,
  Progress,
  ServiceInstallForm,
  SettingsError,
  StatusDot,
  UpdateStrip,
  normalizedProgress,
  type ComponentJob,
} from '../index';

function renderWithI18n(ui: ReactNode) {
  return render(<I18nProvider>{ui}</I18nProvider>);
}

const runningJob: ComponentJob = {
  action: 'install',
  state: 'running',
  phase: 'downloading',
  progress: 0.42,
};

describe('componentes base', () => {
  it('normalizedProgress trata -1, NaN y valores fuera de rango como desconocidos', () => {
    expect(normalizedProgress({ progress: -1 })).toBeNull();
    expect(normalizedProgress({ progress: Number.NaN })).toBeNull();
    expect(normalizedProgress({ progress: 1.4 })).toBeNull();
    expect(normalizedProgress({ progress: null })).toBeNull();
    expect(normalizedProgress({ progress: 0 })).toBe(0);
    expect(normalizedProgress({ progress: 1 })).toBe(1);
  });

  it('Badge y StatusDot usan claves de idioma y etiquetas accesibles', () => {
    renderWithI18n(
      <>
        <Badge labelKey="common.default" tone="indigo" titleKey="php.xdebug.tooltip" />
        <StatusDot on labelKey="common.running" />
        <StatusDot on={false} labelKey="common.stopped" />
      </>,
    );

    expect(screen.getByText('Default')).toBeInTheDocument();
    expect(screen.getByTitle('Xdebug is enabled for this version')).toBeInTheDocument();
    expect(screen.getByLabelText('Running')).toHaveAttribute('data-on', 'true');
    expect(screen.getByLabelText('Stopped')).toHaveAttribute('data-on', 'false');
  });

  it('ComponentJobProgress muestra barra, spinner o instalación según el job', () => {
    const { rerender } = renderWithI18n(
      <ComponentJobProgress title="PHP 8.4" job={runningJob} onCancel={() => {}} />,
    );
    expect(screen.getByText('Downloading… 42%')).toBeInTheDocument();
    expect(document.querySelector('progress')).toHaveAttribute('value', '0.42');

    rerender(
      <I18nProvider>
        <ComponentJobProgress title="PHP 8.4" job={{ ...runningJob, progress: -1 }} />
      </I18nProvider>,
    );
    expect(screen.getByText('Downloading…')).toBeInTheDocument();
    expect(screen.getByRole('status')).toHaveClass('spinner');

    rerender(
      <I18nProvider>
        <ComponentJobProgress title="PHP 8.4" job={{ ...runningJob, phase: 'installing' }} />
      </I18nProvider>,
    );
    expect(screen.getByText('Installing…')).toBeInTheDocument();
    expect(document.querySelector('progress')).not.toBeInTheDocument();
  });

  it('ComponentJobFailure usa un texto por defecto y expone sus acciones', () => {
    const onShowLog = vi.fn();
    const onRetry = vi.fn();
    const onDismiss = vi.fn();
    renderWithI18n(
      <ComponentJobFailure
        name="MySQL 8.4"
        job={{ action: 'install', state: 'failed', phase: 'downloading', progress: -1 }}
        onShowLog={onShowLog}
        onRetry={onRetry}
        onDismiss={onDismiss}
      />,
    );

    expect(screen.getByText('Download failed')).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Show Log' }));
    fireEvent.click(screen.getByRole('button', { name: 'Retry' }));
    fireEvent.click(screen.getByRole('button', { name: 'Dismiss' }));
    expect(onShowLog).toHaveBeenCalledOnce();
    expect(onRetry).toHaveBeenCalledOnce();
    expect(onDismiss).toHaveBeenCalledOnce();
  });

  it('Progress colorea la barra según el tono', () => {
    const low = render(<Progress value={20} tone="danger" label="Bajo" />);
    expect(low.container.querySelector('.bg-destructive')).toBeTruthy();
    low.unmount();
    const mid = render(<Progress value={70} tone="warning" label="Medio" />);
    expect(mid.container.querySelector('.bg-amber-500')).toBeTruthy();
    mid.unmount();
    const done = render(<Progress value={100} tone="success" label="Listo" />);
    expect(done.container.querySelector('.bg-emerald-500')).toBeTruthy();
  });

  it('DumpsTimeline formatea dumps y permite ver el crudo', () => {
    const event = {
      id: 'd1', site: 'app-blade', type: 'logs', time: '2026-11-24T20:35:00Z', request: '/lair-dumps-demo',
      message: '<script>var x=1;</script><pre class="sf-dump">"Lair Dumps desde peticion HTTP"</pre>',
    };
    renderWithI18n(<DumpsTimeline events={[event]} />);
    expect(screen.getByText('Views')).toBeTruthy();
    expect(screen.getByText(/"Lair Dumps desde peticion HTTP"/)).toBeTruthy();
    const raw = screen.getByRole('button', { name: 'View raw' });
    fireEvent.click(raw);
    expect(screen.getByText(/var x=1/)).toBeTruthy();
  });

  it('ServiceInstallForm valida el puerto, sugiere otro y envía la línea elegida', () => {
    const onInstall = vi.fn();
    const lines = [{ id: '8.4', label: '8.4 LTS', latest: '8.4.7' }];

    function FormHarness() {
      const [port, setPort] = useState(3306);
      return (
        <ServiceInstallForm
          name="MySQL"
          lines={lines}
          port={port}
          defaultPort={3306}
          suggestedPort={3307}
          onPortChange={setPort}
          onInstall={onInstall}
        />
      );
    }

    renderWithI18n(<FormHarness />);
    const installButton = screen.getByRole('button', { name: 'Install MySQL 8.4.7' });
    const portInput = screen.getByRole('spinbutton');
    expect(installButton).toBeEnabled();

    fireEvent.change(portInput, { target: { value: '80' } });
    expect(screen.getByText('Pick a port between 1024 and 65535')).toBeInTheDocument();
    expect(installButton).toBeDisabled();

    fireEvent.change(portInput, { target: { value: '3307' } });
    expect(screen.getByText('Port 3306 is taken; suggesting 3307')).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Install MySQL 8.4.7' }));
    expect(onInstall).toHaveBeenCalledWith('8.4', 3307);
  });

  it('UpdateStrip y SettingsError respetan plurales y acción de cierre', () => {
    const onDismiss = vi.fn();
    renderWithI18n(
      <>
        <UpdateStrip count={1} />
        <UpdateStrip count={3} />
        <SettingsError message="sin red" onDismiss={onDismiss} />
      </>,
    );

    expect(screen.getByText('1 update available')).toBeInTheDocument();
    expect(screen.getByText('3 updates available')).toBeInTheDocument();
    expect(screen.getByRole('alert')).toHaveTextContent('sin red');
    fireEvent.click(screen.getByRole('button', { name: 'Close' }));
    expect(onDismiss).toHaveBeenCalledOnce();
  });

  it('CheckForUpdatesLine deshabilita Check now mientras comprueba', () => {
    const onCheck = vi.fn();
    renderWithI18n(
      <CheckForUpdatesLine
        checkedAt="hace 5 min"
        checking
        onCheck={onCheck}
        error="manifiesto caducado"
      />,
    );
    expect(screen.getByText('Checking…')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Check now' })).toBeDisabled();
    expect(screen.getByText('manifiesto caducado')).toBeInTheDocument();
    expect(onCheck).not.toHaveBeenCalled();
  });

  it('CodeBlock copia el texto y confirma con una clave traducida', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined);
    Object.defineProperty(navigator, 'clipboard', {
      configurable: true,
      value: { writeText },
    });

    renderWithI18n(<CodeBlock text="DB_HOST=127.0.0.1" />);
    fireEvent.click(screen.getByRole('button', { name: 'Copy' }));
    await waitFor(() => expect(screen.getByRole('button', { name: 'Copied' })).toBeInTheDocument());
    expect(writeText).toHaveBeenCalledWith('DB_HOST=127.0.0.1');
  });

  it('LogViewer cierra con Escape y devuelve el foco al disparador', () => {
    const trigger = document.createElement('button');
    document.body.appendChild(trigger);
    trigger.focus();
    const onClose = vi.fn();

    const view = renderWithI18n(<LogViewer title="Log" text="lineas" onClose={onClose} />);
    const dialog = screen.getByRole('dialog', { name: 'Log' });
    expect(dialog.contains(document.activeElement)).toBe(true);

    fireEvent.keyDown(document, { key: 'Escape' });
    expect(onClose).toHaveBeenCalledOnce();

    view.unmount();
    expect(document.activeElement).toBe(trigger);
    trigger.remove();
  });
});
