import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import App from './App';
import { detectLocale, I18nProvider } from './i18n';
import { platform } from './platform';
import './styles/globals.css';
import './styles/tokens.css';

const container = document.getElementById('root');
if (!container) throw new Error('Root element missing');

// Platform-specific window chrome (traffic lights, caption buttons) is styled
// in styles/app.css from this attribute.
document.documentElement.dataset.platform = platform;

// shadcn tokens use a `.dark` class; keep it in sync with the OS preference so
// dark mode works without adding a theme switch to every pane.
const darkQuery = window.matchMedia?.('(prefers-color-scheme: dark)');
const applyDark = () => document.documentElement.classList.toggle('dark', Boolean(darkQuery?.matches));
applyDark();
darkQuery?.addEventListener?.('change', applyDark);

// I18nProvider keeps this in sync afterwards; set the initial value before
// the first paint so screen readers get the right language straight away.
document.documentElement.lang = detectLocale();

createRoot(container).render(
  <StrictMode>
    <I18nProvider>
      <App />
    </I18nProvider>
  </StrictMode>,
);
