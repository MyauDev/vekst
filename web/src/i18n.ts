/**
 * The smallest thing that satisfies "every string has a key, in en and ru".
 *
 * Not a library. `add-web-app-shell` (5.1) establishes the token layer and the
 * real i18n setup, and everything here is expected to be replaced by it. What
 * must survive that replacement is the discipline: no English sentence is
 * written inline in a component, and the backend never sends one either -- it
 * returns codes, and translating them is the client's job.
 */
export type Locale = "en" | "ru";

const messages = {
  en: {
    "app.title": "Vekst",
    "app.tagline": "Management reporting for owner-run companies.",

    "signIn.heading": "Sign in",
    "signIn.google": "Continue with Google",
    "signIn.blurb": "Sign-in is by Google account.",

    "signedIn.greeting": "Signed in",
    "signedIn.signOut": "Sign out",
    // Change 1.1 adds organisations and memberships. Until it lands a signed-in
    // person deliberately belongs to nothing, and saying so is what keeps the
    // empty screen from reading as a bug.
    "signedIn.noOrganisation":
      "You do not belong to an organisation yet. Organisations arrive in a later change; there is nothing to show here until then.",

    "error.auth_not_configured": "Sign-in is not configured on this server.",
    "error.invalid_flow": "That sign-in link has expired or was already used. Please try again.",
    "error.invalid_token": "Google could not be verified. Please try again.",
    "error.unverified_email": "Your Google account's email address is not verified.",
    "error.email_taken": "That email address already belongs to another account.",
    "error.internal_error": "Something went wrong. Please try again.",
    "error.unknown": "Sign-in failed. Please try again.",
  },
  ru: {
    "app.title": "Vekst",
    "app.tagline": "Управленческая отчётность для собственников бизнеса.",

    "signIn.heading": "Вход",
    "signIn.google": "Войти через Google",
    "signIn.blurb": "Вход выполняется через аккаунт Google.",

    "signedIn.greeting": "Вы вошли",
    "signedIn.signOut": "Выйти",
    "signedIn.noOrganisation":
      "Вы пока не состоите ни в одной организации. Организации появятся в следующем изменении; до этого здесь нечего показать.",

    "error.auth_not_configured": "Вход не настроен на этом сервере.",
    "error.invalid_flow": "Ссылка для входа устарела или уже использована. Попробуйте ещё раз.",
    "error.invalid_token": "Не удалось проверить ответ Google. Попробуйте ещё раз.",
    "error.unverified_email": "Адрес электронной почты вашего аккаунта Google не подтверждён.",
    "error.email_taken": "Этот адрес электронной почты уже принадлежит другому аккаунту.",
    "error.internal_error": "Что-то пошло не так. Попробуйте ещё раз.",
    "error.unknown": "Не удалось войти. Попробуйте ещё раз.",
  },
} as const;

export type MessageKey = keyof (typeof messages)["en"];

/** Falls back to English for any locale the product does not carry. */
export function resolveLocale(raw: string | undefined): Locale {
  return raw?.toLowerCase().startsWith("ru") ? "ru" : "en";
}

export function t(key: MessageKey, locale: Locale = resolveLocale(navigator?.language)): string {
  return messages[locale][key];
}

/**
 * Maps a code from the backend onto a message. The backend returns codes and
 * never sentences, so an unrecognised one still has to render as something a
 * person can read.
 */
export function authErrorMessage(code: string | null, locale?: Locale): string | null {
  if (!code) return null;
  const key = `error.${code}` as MessageKey;
  return key in messages.en ? t(key, locale) : t("error.unknown", locale);
}
