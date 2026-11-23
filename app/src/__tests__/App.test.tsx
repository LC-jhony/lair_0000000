import { fireEvent, render, screen } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import App from '../App';
import { I18nProvider } from '../i18n';

function clickPane(name: string) {
  fireEvent.click(screen.getByRole('button', { name }));
}

describe('dashboard de escritorio', () => {
  const originalSearch = window.location.search;

  beforeEach(() => {
    // ?fake=1 uses the in-process daemon fixture instead of the real bridge.
    window.history.replaceState({}, '', '/?fake=1');
  });

  afterEach(() => {
    window.history.replaceState({}, '', originalSearch || '/');
  });

  it('muestra el estado real en cada sección del sidebar', async () => {
    render(
      <I18nProvider>
        <App />
      </I18nProvider>,
    );

    // Overview comes from the daemon status, not from a placeholder.
    expect(await screen.findByText('Running services')).toBeInTheDocument();
    expect(screen.queryByText('settings.pane.comingSoon')).not.toBeInTheDocument();

    clickPane('Services');
    expect((await screen.findAllByText('mariadb')).length).toBeGreaterThan(0);
    expect(screen.getAllByText('Active').length).toBeGreaterThan(0);

    clickPane('Runtimes');
    expect((await screen.findAllByText('PHP 8.4')).length).toBeGreaterThan(0);
    expect((await screen.findAllByText('Node v26.13.1')).length).toBeGreaterThan(0);

    clickPane('Databases');
    expect((await screen.findAllByText('mariadb')).length).toBeGreaterThan(0);

    clickPane('Sites');
    expect((await screen.findAllByText('demo')).length).toBeGreaterThan(0);
    expect(screen.getByRole('button', { name: 'Lair configuration' })).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Park directory' })).toBeNull();
    expect(screen.queryByRole('button', { name: 'Link site' })).toBeNull();

    clickPane('System');
    expect(await screen.findByText('Update catalog')).toBeInTheDocument();
    expect(await screen.findByText('Logs')).toBeInTheDocument();
    expect(await screen.findByText('php-8.4')).toBeInTheDocument();
  });
});
