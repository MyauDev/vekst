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
    "app.title": "Veekst",
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

    // The state vocabulary, docs/DESIGN.md §7. Three axes, nine states, one
    // word each. Keys are axis-prefixed because "blocked" is not one word in
    // Russian: a report is Заблокирован and a row is Заблокировано.
    "state.batch.pending": "Pending",
    "state.batch.parsing": "Parsing",
    "state.batch.rejected": "Rejected",
    "state.batch.imported": "Imported",
    "state.report.ready": "Ready",
    "state.report.partial": "Partial",
    "state.report.blocked": "Blocked",
    "state.row.classified": "Classified",
    "state.row.needsReview": "Needs review",
    "state.row.blocked": "Blocked",

    "nav.imports": "Imports",
    "nav.review": "Review",
    "nav.reports": "Reports",
    "nav.awaitingReview": "awaiting review",
    "topbar.organisation": "Organisation",
    "topbar.entity": "Entity",
    "topbar.period": "Period",
    "topbar.language": "Language",
    "topbar.theme": "Theme",
    "action.retry": "Try again",

    // Empty states. Written as what a customer meets on their first morning,
    // not as notes about unbuilt work -- they survive into the finished screens.
    "empty.imports.title": "No files imported yet",
    "empty.imports.detail":
      "Upload a bank statement or a ledger export to begin. Every file is validated before anything is stored, and a file that fails stores nothing.",
    "empty.review.title": "Nothing to review",
    "empty.review.detail":
      "Transactions the engine could not classify confidently appear here, grouped by counterparty. An empty queue means every imported row has a category.",
    "empty.reports.title": "No report yet",
    "empty.reports.detail":
      "The Management P&L is built from imported transactions. Import a bank statement or a ledger export and the report opens here.",
    "empty.batch.title": "Batch not found",
    "empty.batch.detail": "This import no longer exists, or it belongs to another organisation.",
    "landing.cta": "Sign in",

    "landing.promise": "Know what your business actually earned.",
    "landing.sub":
      "Vekst turns your bank statements and your accountant's ledger into a management P&L you can open, line by line, down to the transaction.",
    "landing.problem.title": "Your bank and your accountant disagree, and both are right",
    "landing.problem.body":
      "A bank statement says when money moved. A ledger says when it was earned. Neither is a management report, and reconciling them by hand each month is where the evening goes — so the numbers arrive late, or they arrive rounded, or they arrive as a feeling.",
    "landing.how.title": "Three steps, and the report opens",
    "landing.how.1.title": "Upload what you already have",
    "landing.how.1.body":
      "Bank exports and ledger exports, CSV or XLSX. Every file is validated before anything is stored — and a file that fails stores nothing, with the errors listed by the line number in your original file.",
    "landing.how.2.title": "The engine classifies it",
    "landing.how.2.body":
      "Deterministic rules first, and your own corrections remembered. Anything it is not sure about goes to a review queue instead of being guessed.",
    "landing.how.3.title": "Read the report",
    "landing.how.3.body":
      "Categories down, months across. Every figure opens to the transactions behind it, each carrying the rule that classified it and how confident it was.",
    "landing.product.title": "This is the report",
    "landing.product.body":
      "Not a screenshot — the same component the application renders, on sample figures. The basis is stated where you read it, a line that cannot be computed honestly says so instead of guessing, and the reconciliation proves nothing was dropped.",
    "landing.close.title": "See it on your own numbers",
    "landing.rights": "Management reporting for owner-run companies.",

    "imports.title": "Imports",
    "imports.upload": "Upload a file",
    "imports.chooseSource": "Where is this file from?",
    "imports.source.ledger": "Ledger",
    "imports.source.bank": "Bank",
    "imports.sourceNote":
      "This decides the accounting basis and cannot be read from the file. It is not optional.",
    "imports.col.file": "File",
    "imports.col.source": "Source",
    "imports.col.state": "State",
    "imports.col.period": "Period",
    "imports.col.uploaded": "Uploaded",
    "imports.counts.imported": "Rows imported",
    "imports.counts.duplicates": "Duplicates skipped",
    "imports.counts.transfers": "Internal transfers",
    "imports.counts.matches": "Matches proposed",
    "batch.rejected.balance_mismatch":
      "Opening balance plus movements does not equal the declared closing balance. Nothing from this file was stored.",
    "batch.errors.title": "Validation errors",
    "batch.errors.line": "Line",
    "batch.errors.download": "Download the list",
    "batch.balance.title": "Balance check",
    "batch.balance.opening": "Opening",
    "batch.balance.movements": "Movements",
    "batch.balance.closing": "Computed closing",
    "batch.balance.declared": "Declared closing",
    "error.amount_unparseable": "The amount could not be read as a number.",
    "error.date_implausible": "The date does not exist.",
    "error.currency_unknown": "Not a valid ISO-4217 currency code.",
    "error.debit_and_credit_both_set": "Debit and credit are both filled in.",
    "error.description_missing": "The description is empty.",

    "report.title": "Management P&L",
    "report.category": "Category",
    "report.total": "Total",
    "report.percentOfRevenue": "% of revenue",
    "report.net": "Net result",
    "report.basis.cash": "Cash-basis",
    "report.basis.accrual": "Accrual",
    "report.basis.note": "derived from the source of the data, not chosen",
    "report.blocked": "Blocked",
    "report.blocked.mixed_sources_no_match":
      "Drawn from both ledger and bank data with no confirmed match. Computing it would count an invoice and its payment twice.",
    "recon.title": "Reconciliation",
    "recon.opening": "Opening",
    "recon.in": "In",
    "recon.out": "Out",
    "recon.transfers": "Transfers",
    "recon.closing": "Closing",
    "stat.revenue": "Revenue",
    "stat.expenses": "Expenses",
    "stat.net": "Net result",
    "stat.unreviewed": "Awaiting review",
    "drilldown.close": "Close",
    "drilldown.date": "Date",
    "drilldown.description": "Description",
    "drilldown.amount": "Amount",
    "drilldown.category": "Category",
    "drilldown.layer": "Layer",
    "drilldown.confidence": "Confidence",
    "drilldown.evidence": "Match",
    "drilldown.unavailable":
      "Transactions for this figure are not in the sample data. The figure itself is real.",
    "drilldown.provenance": "Taxonomy {taxonomy} · ruleset {ruleset} · engine {engine}",
  },
  ru: {
    "app.title": "Veekst",
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

    // Draft, by a non-native writer. docs/DESIGN.md §12 Q1 -- confirm before
    // the catalogue is frozen.
    "state.batch.pending": "Ожидает",
    "state.batch.parsing": "Обработка",
    "state.batch.rejected": "Отклонён",
    "state.batch.imported": "Импортирован",
    "state.report.ready": "Готов",
    "state.report.partial": "Частично",
    "state.report.blocked": "Заблокирован",
    "state.row.classified": "Классифицировано",
    "state.row.needsReview": "На проверку",
    "state.row.blocked": "Заблокировано",

    "nav.imports": "Импорт",
    "nav.review": "Проверка",
    "nav.reports": "Отчёты",
    "nav.awaitingReview": "на проверке",
    "topbar.organisation": "Организация",
    "topbar.entity": "Юр. лицо",
    "topbar.period": "Период",
    "topbar.language": "Язык",
    "topbar.theme": "Тема",
    "action.retry": "Повторить",

    "empty.imports.title": "Файлы ещё не импортированы",
    "empty.imports.detail":
      "Загрузите банковскую выписку или выгрузку из учёта. Каждый файл проверяется до сохранения: файл с ошибкой не сохраняет ничего.",
    "empty.review.title": "Нечего проверять",
    "empty.review.detail":
      "Здесь появляются операции, которые движок не смог классифицировать уверенно, сгруппированные по контрагенту. Пустая очередь означает, что у каждой импортированной строки есть категория.",
    "empty.reports.title": "Отчёта пока нет",
    "empty.reports.detail":
      "Управленческий ОПиУ строится из импортированных операций. Загрузите выписку или выгрузку — и отчёт откроется здесь.",
    "empty.batch.title": "Пакет не найден",
    "empty.batch.detail": "Этот импорт больше не существует или принадлежит другой организации.",
    "landing.cta": "Войти",

    "landing.promise": "Узнайте, сколько бизнес заработал на самом деле.",
    "landing.sub":
      "Vekst превращает банковские выписки и учётные данные бухгалтера в управленческий отчёт, который можно раскрыть построчно — вплоть до операции.",
    "landing.problem.title": "Банк и бухгалтер противоречат друг другу, и оба правы",
    "landing.problem.body":
      "Выписка показывает, когда деньги пришли. Учёт показывает, когда они заработаны. Ни то ни другое не является управленческим отчётом, а сверять их вручную каждый месяц — это потерянный вечер. Поэтому цифры приходят поздно, округлённо или на уровне ощущений.",
    "landing.how.title": "Три шага — и отчёт открыт",
    "landing.how.1.title": "Загрузите то, что уже есть",
    "landing.how.1.body":
      "Банковские и учётные выгрузки, CSV или XLSX. Каждый файл проверяется до сохранения: файл с ошибкой не сохраняет ничего, а ошибки перечислены по номерам строк вашего исходного файла.",
    "landing.how.2.title": "Движок классифицирует данные",
    "landing.how.2.body":
      "Сначала детерминированные правила, затем — запомненные ваши исправления. Всё, в чём движок не уверен, попадает в очередь проверки, а не угадывается.",
    "landing.how.3.title": "Читайте отчёт",
    "landing.how.3.body":
      "Категории по строкам, месяцы по столбцам. Любая сумма раскрывается до операций, и каждая несёт правило, которое её классифицировало, и уверенность.",
    "landing.product.title": "Вот такой отчёт",
    "landing.product.body":
      "Не скриншот — тот же компонент, что рендерит приложение, на демонстрационных данных. Метод учёта указан там, где вы читаете, строка, которую нельзя посчитать честно, говорит об этом вместо догадки, а сверка доказывает, что ничего не потеряно.",
    "landing.close.title": "Посмотрите на своих данных",
    "landing.rights": "Управленческая отчётность для собственников бизнеса.",

    "imports.title": "Импорт",
    "imports.upload": "Загрузить файл",
    "imports.chooseSource": "Откуда этот файл?",
    "imports.source.ledger": "Учёт",
    "imports.source.bank": "Банк",
    "imports.sourceNote":
      "Определяет метод учёта и не может быть прочитан из файла. Указание обязательно.",
    "imports.col.file": "Файл",
    "imports.col.source": "Источник",
    "imports.col.state": "Статус",
    "imports.col.period": "Период",
    "imports.col.uploaded": "Загружен",
    "imports.counts.imported": "Строк импортировано",
    "imports.counts.duplicates": "Дубликатов пропущено",
    "imports.counts.transfers": "Внутренних переводов",
    "imports.counts.matches": "Предложено сопоставлений",
    "batch.rejected.balance_mismatch":
      "Входящий остаток плюс обороты не равны заявленному исходящему остатку. Из этого файла ничего не сохранено.",
    "batch.errors.title": "Ошибки проверки",
    "batch.errors.line": "Строка",
    "batch.errors.download": "Скачать список",
    "batch.balance.title": "Проверка баланса",
    "batch.balance.opening": "Входящий остаток",
    "batch.balance.movements": "Обороты",
    "batch.balance.closing": "Расчётный остаток",
    "batch.balance.declared": "Заявленный остаток",
    "error.amount_unparseable": "Сумму не удалось прочитать как число.",
    "error.date_implausible": "Такой даты не существует.",
    "error.currency_unknown": "Недопустимый код валюты ISO-4217.",
    "error.debit_and_credit_both_set": "Заполнены и дебет, и кредит.",
    "error.description_missing": "Описание пустое.",

    "report.title": "Управленческий ОПиУ",
    "report.category": "Категория",
    "report.total": "Итого",
    "report.percentOfRevenue": "% от выручки",
    "report.net": "Финансовый результат",
    "report.basis.cash": "Кассовый метод",
    "report.basis.accrual": "Метод начисления",
    "report.basis.note": "определяется источником данных, а не выбором",
    "report.blocked": "Заблокировано",
    "report.blocked.mixed_sources_no_match":
      "Строка собрана из учётных и банковских данных без подтверждённого сопоставления. Расчёт учёл бы счёт и его оплату дважды.",
    "recon.title": "Сверка",
    "recon.opening": "Входящий остаток",
    "recon.in": "Поступления",
    "recon.out": "Списания",
    "recon.transfers": "Переводы",
    "recon.closing": "Исходящий остаток",
    "stat.revenue": "Выручка",
    "stat.expenses": "Расходы",
    "stat.net": "Финансовый результат",
    "stat.unreviewed": "На проверке",
    "drilldown.close": "Закрыть",
    "drilldown.date": "Дата",
    "drilldown.description": "Описание",
    "drilldown.amount": "Сумма",
    "drilldown.category": "Категория",
    "drilldown.layer": "Слой",
    "drilldown.confidence": "Уверенность",
    "drilldown.evidence": "Сопоставление",
    "drilldown.unavailable":
      "Операции по этой сумме отсутствуют в демонстрационных данных. Сама сумма реальна.",
    "drilldown.provenance": "Таксономия {taxonomy} · правила {ruleset} · движок {engine}",
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
