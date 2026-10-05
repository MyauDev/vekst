import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";

import { authErrorMessage, messages, resolveLocale, t } from "./i18n";

describe("i18n", () => {
  it("carries every key in both languages", () => {
    // The failure this catches is a string added in English and forgotten in
    // Russian, which renders as `undefined` rather than as an error.
    const en = t("signIn.google", "en");
    const ru = t("signIn.google", "ru");
    expect(en).toBeTruthy();
    expect(ru).toBeTruthy();
    expect(ru).not.toBe(en);
  });

  it("holds exactly the same keys in en and ru", () => {
    // The catalogue is typed against `en`, so a key missing from `ru` is not a
    // compile error -- it is `undefined` on screen for every Russian reader.
    const en = Object.keys(messages.en).sort();
    const ru = Object.keys(messages.ru).sort();
    expect(ru.filter((k) => !en.includes(k)), "in ru, not in en").toEqual([]);
    expect(en.filter((k) => !ru.includes(k)), "in en, not in ru").toEqual([]);
    for (const [key, value] of Object.entries(messages.ru)) {
      expect(value, `empty ru value for ${key}`).toBeTruthy();
    }
  });

  it("never spells the brand Vekst on screen", () => {
    // `Veekst` on screen, `vekst` in every identifier. Only a check that
    // knows which is which keeps the two from drifting back together.
    const html = readFileSync(
      join(dirname(fileURLToPath(import.meta.url)), "..", "index.html"),
      "utf8",
    );
    const surfaces: [string, string][] = [
      ...Object.entries(messages.en).map(([k, v]): [string, string] => [`en ${k}`, v]),
      ...Object.entries(messages.ru).map(([k, v]): [string, string] => [`ru ${k}`, v]),
      ["index.html", html],
    ];
    for (const [where, text] of surfaces) {
      expect(text, where).not.toMatch(/Vekst/);
    }
    expect(t("app.title", "en")).toBe("Veekst");
  });

  it("falls back to English for a locale the product does not carry", () => {
    expect(resolveLocale("nb-NO")).toBe("en");
    expect(resolveLocale(undefined)).toBe("en");
    expect(resolveLocale("ru-RU")).toBe("ru");
  });

  it("translates every error code the auth routes can return", () => {
    for (const code of [
      "auth_not_configured",
      "invalid_flow",
      "invalid_token",
      "unverified_email",
      "email_taken",
      "internal_error",
    ]) {
      const message = authErrorMessage(code, "en");
      expect(message, `no message for ${code}`).toBeTruthy();
      expect(message).not.toBe(t("error.unknown", "en"));
    }
  });

  it("renders an unrecognised code rather than nothing", () => {
    // The backend returns codes; a code this build has never heard of must
    // still reach the person as something readable.
    expect(authErrorMessage("a_code_from_the_future", "en")).toBe(t("error.unknown", "en"));
    expect(authErrorMessage(null)).toBeNull();
  });
});
