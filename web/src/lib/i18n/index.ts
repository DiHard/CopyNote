import { en } from "./en";
import { ru } from "./ru";

const dictionaries: Record<string, Record<string, string>> = { en, ru };

// Default to system language immediately so the first render is
// already in the correct locale — before loadSettings() resolves.
let currentLocale = (() => {
  const short = (navigator.language || "en").slice(0, 2).toLowerCase();
  return short in dictionaries ? short : "en";
})();

/**
 * Set the active locale. Falls back to "en" if the locale isn't
 * found. Accepts full BCP-47 tags like "ru-RU" — only the first
 * two-letter prefix is used.
 */
export function setLocale(locale: string): void {
  const short = locale.slice(0, 2).toLowerCase();
  currentLocale = short in dictionaries ? short : "en";
}

/**
 * Resolve the system locale from `navigator.language`.
 * Returns "en" if the browser language isn't in our dictionaries.
 */
export function systemLocale(): string {
  const short = (navigator.language || "en").slice(0, 2).toLowerCase();
  return short in dictionaries ? short : "en";
}

/**
 * Translate a key using the active locale's dictionary.
 * Supports simple `{placeholder}` interpolation via an optional
 * params object: `t("list.error", { error: "timeout" })`.
 * Returns the key itself if no translation is found (makes missing
 * keys obvious during development).
 */
export function t(key: string, params?: Record<string, string>): string {
  let str = dictionaries[currentLocale]?.[key] ?? dictionaries.en[key] ?? key;
  if (params) {
    for (const [k, v] of Object.entries(params)) {
      str = str.replace(`{${k}}`, v);
    }
  }
  return str;
}

/** Get the current locale code (e.g., "en", "ru"). */
export function getLocale(): string {
  return currentLocale;
}

/** List of available locales with display names. */
export const availableLocales = [
  { code: "system", label: "System" },
  { code: "en", label: "English" },
  { code: "ru", label: "Русский" },
];
