import type { Locale, Messages, Translator } from './types';

/**
 * Creates a translator for one locale. Missing keys return the key itself so a
 * forgotten translation is visible in the UI and detectable in tests.
 */
export function createTranslator(locale: Locale, messages: Messages): Translator {
  const plural = new Intl.PluralRules(locale);

  return (key, params = {}) => {
    let template = messages[key];

    if (template === undefined && typeof params.count === 'number') {
      const form = plural.select(params.count);
      template = messages[`${key}.${form}`] ?? messages[`${key}.other`];
    }

    if (template === undefined) return key;

    return template.replace(/\{\{(\w+)\}\}/g, (match, name: string) => {
      const value = params[name];
      return value === undefined ? match : String(value);
    });
  };
}
