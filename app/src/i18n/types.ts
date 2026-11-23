export type Locale = 'en' | 'es' | 'cs';

export const LOCALES: readonly Locale[] = ['en', 'es', 'cs'];

/** Flat JSON: "common.install": "Install". Plurals use .one/.few/.other suffixes. */
export type Messages = Record<string, string>;

export type TranslateParams = Record<string, string | number | undefined> & {
  count?: number;
};

export type Translator = (key: string, params?: TranslateParams) => string;
