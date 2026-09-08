/**
 * Theme and locale: the two things that belong to the reader rather than to the
 * figure they are reading.
 *
 * Neither goes in the URL. A shared drill-down link says which figures to look
 * at; the language and theme it renders in are the recipient's
 * (`add-web-experience` design D7). Both therefore live in `localStorage` and
 * fall back to what the browser already tells us.
 *
 * `useSyncExternalStore` rather than a context: these are two values read by
 * almost every component and written by two controls, which is exactly the
 * shape that makes a provider tree expensive and a store cheap.
 */
import { useSyncExternalStore } from "react";

/**
 * Three states, not two. "system" is the default and is distinct from an
 * explicit "light" -- a reader who has never chosen follows their OS, and a
 * reader who chose light stays light when their OS flips at sunset.
 */
export type ThemeChoice = "system" | "light" | "dark";
export type Locale = "en" | "ru";

const THEME_KEY = "veekst.theme";
const LOCALE_KEY = "veekst.locale";

const listeners = new Set<() => void>();
function emit() {
  for (const l of listeners) l();
}
function subscribe(l: () => void) {
  listeners.add(l);
  return () => listeners.delete(l);
}

/** Storage can throw, not merely return null: Safari in private mode, and any
 *  browser told to block site data. A preference is never worth a blank page. */
function read(key: string): string | null {
  try {
    return localStorage.getItem(key);
  } catch {
    return null;
  }
}
function write(key: string, value: string) {
  try {
    localStorage.setItem(key, value);
  } catch {
    /* the choice still applies for this page view */
  }
}

let theme: ThemeChoice = "system";
let locale: Locale = "en";

export function resolveLocale(raw: string | undefined): Locale {
  return raw?.toLowerCase().startsWith("ru") ? "ru" : "en";
}

/**
 * Applies the current choices to the document, which is what the token layer
 * reads. `data-theme` is *absent* for "system", so the stylesheet's
 * `prefers-color-scheme` block governs -- setting `data-theme="system"` would
 * match neither selector and silently pin light.
 */
function apply() {
  if (typeof document === "undefined") return;
  const root = document.documentElement;
  if (theme === "system") root.removeAttribute("data-theme");
  else root.setAttribute("data-theme", theme);
  root.setAttribute("lang", locale);
}

/** Reads what was stored before the first render. Safe to call more than once. */
export function initPreferences() {
  const t = read(THEME_KEY);
  if (t === "light" || t === "dark" || t === "system") theme = t;

  const l = read(LOCALE_KEY);
  if (l === "en" || l === "ru") locale = l;
  else if (typeof navigator !== "undefined") locale = resolveLocale(navigator.language);

  apply();
}

export function setTheme(next: ThemeChoice) {
  theme = next;
  write(THEME_KEY, next);
  apply();
  emit();
}

export function setLocale(next: Locale) {
  locale = next;
  write(LOCALE_KEY, next);
  apply();
  emit();
}

export function useTheme(): [ThemeChoice, (next: ThemeChoice) => void] {
  const value = useSyncExternalStore(
    subscribe,
    () => theme,
    () => theme,
  );
  return [value, setTheme];
}

export function useLocale(): [Locale, (next: Locale) => void] {
  const value = useSyncExternalStore(
    subscribe,
    () => locale,
    () => locale,
  );
  return [value, setLocale];
}

/**
 * What is actually on screen once "system" is resolved. The token sheet needs
 * it to label which palette is being shown; ordinary components do not, because
 * the tokens have already resolved by the time they render.
 */
export function resolvedTheme(choice: ThemeChoice): "light" | "dark" {
  if (choice !== "system") return choice;
  if (typeof window === "undefined" || typeof window.matchMedia !== "function") return "light";
  return window.matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light";
}
