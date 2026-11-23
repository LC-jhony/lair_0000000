import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
  type ReactNode,
} from 'react';
import en from '../../lang/en/app.json';
import es from '../../lang/es/app.json';
import cs from '../../lang/cs/app.json';
import { createTranslator } from './translate';
import { LOCALES, type Locale, type Messages, type Translator } from './types';

const catalogs: Record<Locale, Messages> = {
  en: en as Messages,
  es: es as Messages,
  cs: cs as Messages,
};

const STORAGE_KEY = 'lair.locale';

function isLocale(value: unknown): value is Locale {
  return typeof value === 'string' && (LOCALES as readonly string[]).includes(value);
}

/** stored preference → browser language → English. */
export function detectLocale(): Locale {
  if (typeof window === 'undefined') return 'en';
  try {
    const stored = window.localStorage?.getItem(STORAGE_KEY);
    if (isLocale(stored)) return stored;
  } catch {
    // Storage may be unavailable (private mode, file:// restrictions).
  }
  const browser = window.navigator?.language?.slice(0, 2).toLowerCase();
  return isLocale(browser) ? browser : 'en';
}

interface I18nValue {
  locale: Locale;
  setLocale: (locale: Locale) => void;
  t: Translator;
}

const I18nContext = createContext<I18nValue | null>(null);

export interface I18nProviderProps {
  children: ReactNode;
  initialLocale?: Locale;
}

export function I18nProvider({ children, initialLocale }: I18nProviderProps) {
  const [locale, setLocaleState] = useState<Locale>(() => initialLocale ?? detectLocale());

  useEffect(() => {
    if (typeof document !== 'undefined') document.documentElement.lang = locale;
  }, [locale]);

  const setLocale = useCallback((next: Locale) => {
    setLocaleState(next);
    try {
      if (typeof window !== 'undefined') window.localStorage?.setItem(STORAGE_KEY, next);
    } catch {
      // The selected language still applies for this session.
    }
  }, []);

  const value = useMemo<I18nValue>(
    () => ({ locale, setLocale, t: createTranslator(locale, catalogs[locale]) }),
    [locale, setLocale],
  );
  return <I18nContext.Provider value={value}>{children}</I18nContext.Provider>;
}

export function useI18n(): I18nValue {
  const value = useContext(I18nContext);
  if (!value) throw new Error('useI18n must be used inside <I18nProvider>');
  return value;
}

export function useT(): Translator {
  return useI18n().t;
}
