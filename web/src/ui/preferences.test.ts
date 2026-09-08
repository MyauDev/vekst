import { beforeEach, describe, expect, it } from "vitest";

import { initPreferences, setLocale, setTheme, resolveLocale, resolvedTheme } from "./preferences";

describe("theme", () => {
  beforeEach(() => {
    localStorage.clear();
    document.documentElement.removeAttribute("data-theme");
    initPreferences();
  });

  it("sets no attribute for the system default, so the media query governs", () => {
    setTheme("system");
    // Not `data-theme="system"`: that matches neither stylesheet selector and
    // would silently pin the light palette.
    expect(document.documentElement.hasAttribute("data-theme")).toBe(false);
  });

  it("stamps an explicit choice so it wins over the system preference", () => {
    setTheme("dark");
    expect(document.documentElement.getAttribute("data-theme")).toBe("dark");
    setTheme("light");
    expect(document.documentElement.getAttribute("data-theme")).toBe("light");
  });

  it("survives a reload", () => {
    setTheme("dark");
    document.documentElement.removeAttribute("data-theme");
    initPreferences();
    expect(document.documentElement.getAttribute("data-theme")).toBe("dark");
  });

  it("resolves system to a concrete palette without matchMedia", () => {
    // jsdom has no matchMedia. The token sheet labels which palette is showing,
    // and it must not throw where the shim is absent.
    expect(resolvedTheme("system")).toBe("light");
    expect(resolvedTheme("dark")).toBe("dark");
  });
});

describe("locale", () => {
  beforeEach(() => {
    localStorage.clear();
    initPreferences();
  });

  it("is a stored preference, not a URL parameter", () => {
    setLocale("ru");
    expect(document.documentElement.getAttribute("lang")).toBe("ru");
    expect(window.location.search).toBe("");
  });

  it("falls back to English for a language the product does not carry", () => {
    expect(resolveLocale("de-DE")).toBe("en");
    expect(resolveLocale("ru-RU")).toBe("ru");
    expect(resolveLocale(undefined)).toBe("en");
  });
});
