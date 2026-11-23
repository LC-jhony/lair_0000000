import { fireEvent, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, it } from 'vitest';
import cs from '../../../lang/cs/app.json';
import en from '../../../lang/en/app.json';
import es from '../../../lang/es/app.json';
import { I18nProvider, useI18n } from '../I18nProvider';
import { createTranslator } from '../translate';
import type { Messages } from '../types';

function keys(object: Record<string, unknown>, prefix = ''): string[] {
  return Object.entries(object).flatMap(([key, value]) => {
    const path = prefix ? `${prefix}.${key}` : key;
    return value && typeof value === 'object' && !Array.isArray(value)
      ? keys(value as Record<string, unknown>, path)
      : [path];
  });
}

function LocaleProbe() {
  const { locale, setLocale, t } = useI18n();
  return (
    <div>
      <span data-testid="locale">{locale}</span>
      <span data-testid="title">{t('lair.config.title')}</span>
      <button type="button" onClick={() => setLocale('es')}>switch</button>
    </div>
  );
}

describe('i18n', () => {
  afterEach(() => {
    window.localStorage.clear();
    document.documentElement.lang = '';
  });

  it('tiene exactamente las mismas claves en en, es y cs', () => {
    const base = keys(en).sort();
    expect(keys(es).sort()).toEqual(base);
    expect(keys(cs).sort()).toEqual(base);
    expect(base.length).toBeGreaterThan(100);
  });

  it('interpola parámetros y deja los desconocidos visibles', () => {
    const t = createTranslator('en', en as Messages);
    expect(t('common.downloadingPercent', { percent: 42 })).toBe('Downloading… 42%');
    expect(t('common.checkedAgo', {})).toBe('Checked {{time}}');
  });

  it('selecciona el plural según el idioma', () => {
    expect(createTranslator('en', en as Messages)('common.updatesAvailable', { count: 1 })).toBe(
      '1 update available',
    );
    expect(createTranslator('en', en as Messages)('common.updatesAvailable', { count: 3 })).toBe(
      '3 updates available',
    );
    expect(createTranslator('cs', cs as Messages)('common.updatesAvailable', { count: 3 })).toBe(
      '3 dostupné aktualizace',
    );
  });

  it('devuelve la clave cuando no existe la traducción', () => {
    const t = createTranslator('es', es as Messages);
    expect(t('php.noExiste')).toBe('php.noExiste');
  });

  it('solo interpola marcadores con doble llave', () => {
    const t = createTranslator('en', {
      'demo.simple': 'Hola {name}',
      'demo.double': 'Hola {{name}}',
    } as Messages);
    expect(t('demo.simple', { name: 'Ana' })).toBe('Hola {name}');
    expect(t('demo.double', { name: 'Ana' })).toBe('Hola Ana');

    for (const catalog of [en, es, cs]) {
      const text = (catalog as Record<string, string>)['web.securing'];
      expect(text).toMatch(/\{\{name\}\}/);
      expect(text).not.toMatch(/(?<!\{)\{name\}(?!\})/);
      expect(createTranslator('en', catalog as Messages)('web.securing', { name: 'demo' })).toContain('demo');
    }
  });

  it('detecta el idioma guardado, luego el del navegador', () => {
    window.localStorage.setItem('lair.locale', 'cs');
    const first = render(<I18nProvider><LocaleProbe /></I18nProvider>);
    expect(screen.getByTestId('locale').textContent).toBe('cs');
    expect(screen.getByTestId('title').textContent).toBe(cs['lair.config.title']);
    expect(document.documentElement.lang).toBe('cs');
    first.unmount();

    window.localStorage.clear();
    const original = window.navigator.language;
    Object.defineProperty(window.navigator, 'language', { configurable: true, value: 'es-ES' });
    const second = render(<I18nProvider><LocaleProbe /></I18nProvider>);
    expect(screen.getByTestId('locale').textContent).toBe('es');
    expect(screen.getByTestId('title').textContent).toBe(es['lair.config.title']);
    second.unmount();
    Object.defineProperty(window.navigator, 'language', { configurable: true, value: original });
  });

  it('cambia el locale y mantiene <html lang> y la preferencia sincronizados', () => {
    const view = render(<I18nProvider><LocaleProbe /></I18nProvider>);
    expect(screen.getByTestId('locale').textContent).toBe('en');

    fireEvent.click(screen.getByRole('button', { name: 'switch' }));
    expect(screen.getByTestId('locale').textContent).toBe('es');
    expect(screen.getByTestId('title').textContent).toBe(es['lair.config.title']);
    expect(document.documentElement.lang).toBe('es');
    expect(window.localStorage.getItem('lair.locale')).toBe('es');
    view.unmount();
  });
});
