import { describe, expect, it } from "vitest";

import { authErrorMessage, resolveLocale, t } from "./i18n";

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
