/**
 * The period range, remembered across navigation.
 *
 * `router.tsx`'s own comment is still true and unchanged by this file: the
 * period lives in the URL because that is what a shared link is about, and
 * an explicit `?from=...&to=...` always wins -- this module never overrides
 * one, it only answers what a link with *no* period should open to.
 *
 * Before this existed, that fallback was `defaultRange()` unconditionally --
 * the current year to date, every render, forever -- so leaving the Report
 * screen and coming back (even by clicking the rail's own "Report" link)
 * silently discarded whatever range a person had just picked, seconds
 * earlier, on the exact same screen.
 *
 * Same shape as `ui/preferences.ts`, deliberately not folded into it: that
 * file's own header says "neither goes in the URL", which is no longer true
 * of this one the moment the URL carries an explicit range. `getPeriodRange`
 * is the synchronous read `router.tsx`'s `validateSearch` needs -- it runs
 * outside React, the same reason that file also exposes a plain getter
 * alongside its hook.
 */
import { useSyncExternalStore } from "react";
import { defaultRange } from "./period";

export interface PeriodRange {
  from: string;
  to: string;
}

const FROM_KEY = "veekst.period.from";
const TO_KEY = "veekst.period.to";
const PERIOD = /^\d{4}-(0[1-9]|1[0-2])$/;

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

let range: PeriodRange = defaultRange();

/**
 * Reads what was stored before the first render. Safe to call more than
 * once, and idempotent given the same storage: falls all the way back to
 * `defaultRange()` when nothing valid is stored, rather than leaving
 * whatever an earlier call happened to leave in memory -- a corrupted or
 * cleared store is a fact about now, not about the last time this ran.
 */
export function initPeriodRange() {
  const from = read(FROM_KEY);
  const to = read(TO_KEY);
  range = from && PERIOD.test(from) && to && PERIOD.test(to) ? { from, to } : defaultRange();
}

/** The synchronous read `validateSearch` needs -- it runs outside React, on
 *  every navigation, which rules out a hook. */
export function getPeriodRange(): PeriodRange {
  return range;
}

export function setPeriodRange(next: PeriodRange) {
  range = next;
  write(FROM_KEY, next.from);
  write(TO_KEY, next.to);
  emit();
}

export function usePeriodRange(): [PeriodRange, (next: PeriodRange) => void] {
  const value = useSyncExternalStore(
    subscribe,
    () => range,
    () => range,
  );
  return [value, setPeriodRange];
}
