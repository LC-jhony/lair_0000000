import React from 'react';
import { render } from 'ink';
import { I18nProvider } from '../src/i18n';
import type { Locale } from '../src/i18n';
import { App } from './App';
import { connect } from './daemon';

if (!process.stdin.isTTY) {
  console.error('lair tui necesita una terminal interactiva (TTY). Ejecútalo en un terminal real o usa `lair status`.');
  process.exit(2);
}

const locale = (process.env.LAIR_LOCALE ?? 'en') as Locale;
const clientFactory = () => connect();

const app = render(
  <I18nProvider initialLocale={locale}>
    <App clientFactory={clientFactory} />
  </I18nProvider>,
);

process.on('SIGINT', () => app.unmount());
