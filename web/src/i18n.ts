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
    "a11y.skipToContent": "Skip to content",

    "notFound.title": "Page not found",
    "notFound.detail": "There is nothing at this address.",
    "notFound.cta": "Back to Veekst",

    "signIn.heading": "Sign in",
    "signIn.google": "Continue with Google",
    "signIn.blurb": "Sign-in is by Google account.",

    "signedIn.greeting": "Signed in",
    "signedIn.signOut": "Sign out",

    "error.auth_not_configured": "Sign-in is not configured on this server.",
    "error.invalid_flow": "That sign-in link has expired or was already used. Please try again.",
    "error.invalid_token": "Google could not be verified. Please try again.",
    "error.unverified_email": "Your Google account's email address is not verified.",
    "error.email_taken": "That email address already belongs to another account.",
    "error.internal_error": "Something went wrong. Please try again.",
    "error.unknown": "Sign-in failed. Please try again.",

    "error.org_name_required": "Enter a name for the organisation.",
    "error.org_entity_name_required": "Enter a name for the entity.",
    "error.org_unsupported_country": "That country is not supported yet.",
    "error.org_unsupported_currency": "That currency is not supported yet.",
    "error.org_already_a_member": "You already belong to an organisation.",

    // The first-run screen: a signed-in person with no organisation.
    // AppLayout renders it in place of the application shell.
    "firstRun.heading": "Set up your organisation",
    "firstRun.blurb":
      "One organisation, one entity, to start. You can invite others and add more later.",
    "firstRun.name": "Organisation name",
    "firstRun.entityName": "Entity name",
    "firstRun.market": "Country and currency",
    "firstRun.market.by": "Belarus — BYN",
    "firstRun.market.kz": "Kazakhstan — KZT",
    "firstRun.market.pl": "Poland — PLN",
    "firstRun.submit": "Create organisation",
    "firstRun.submitting": "Creating…",

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

    "nav.primary": "Main",
    "nav.home": "Home",
    "nav.imports": "Imports",
    "nav.review": "Review",
    "nav.reports": "Reports",
    "nav.awaitingReview": "awaiting review",
    "topbar.organisation": "Organisation",
    "topbar.entity": "Entity",
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
    "chart.expenses.title": "Top expense categories",
    "chart.moneyflow.title": "Money flow",
    "chart.moneyflow.totalIn": "Total in",
    "chart.moneyflow.other": "Other",
    "chart.netresult.title": "Net result by month",
    "chart.netresult.series": "Net result",
    "chart.revenueExpense.title": "Revenue against expenses",
    "chart.revenueExpense.revenue": "Revenue",
    "chart.revenueExpense.expenses": "Expenses",
    "chart.trend.title": "Category trend",
    "chart.view.chart": "Chart",
    "chart.view.table": "Table",

    "landing.cta": "Sign in",
    "landing.cta.secondary": "See the report",
    "landing.nav.how": "How it works",
    "landing.nav.product": "The report",

    "landing.promise": "Know what your business actually earned.",
    "landing.sub":
      "Veekst turns your bank statements and your accountant's ledger into a management P&L you can open, line by line, down to the transaction.",
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

    "home.title": "Home",
    "home.imports.count": "Batches",
    "home.imports.latest": "Latest",
    "home.review.awaiting": "Awaiting review",
    "home.reports.net": "Net result",
    "home.account.title": "Account",

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
    "batch.rows.title": "Rows",
    "batch.rows.category": "Category",
    "batch.rows.unclassified": "Unclassified",
    "error.amount_unparseable": "The amount could not be read as a number.",
    "error.date_implausible": "The date does not exist.",
    "error.currency_unknown": "Not a valid ISO-4217 currency code.",
    "error.debit_and_credit_both_set": "Debit and credit are both filled in.",
    "error.description_missing": "The description is empty.",

    // add-file-upload's six codes. upload_missing and file_too_large are
    // failure_code values a batch can carry; the other four are RPC-level
    // codes the same handful of calls can answer with.
    "error.upload_missing": "The file never reached us. Please upload it again.",
    "error.file_too_large": "This file is larger than the 25 MB limit.",
    "error.object_store_not_configured": "File upload is not configured on this server.",
    "error.invalid_argument": "That request was not valid.",
    "error.batch_not_found": "This import no longer exists, or it belongs to another organisation.",
    "error.not_a_member": "You do not have access to that organisation.",

    // add-ingest-validation's remaining seven codes (five of the twelve
    // already have keys above, from the earlier mock fixtures).
    "error.replacement_character": "This file contains corrupted text. Re-export it and try again.",
    "error.account_unresolved": "This file's account could not be matched to one on file.",
    "error.balance_mismatch": "Opening balance plus movements does not equal the declared closing balance.",
    "error.row_count_mismatch": "The number of rows does not match what the file declares.",
    "error.period_gap": "A row falls outside the period this file declares.",
    "error.mixed_currency": "This account is on file in a different currency.",
    "error.duplicate_bank_reference": "The same posting appears twice in this file.",

    // The three outcomes.
    "validation.outcome.valid": "Valid",
    "validation.outcome.valid_with_warnings": "Valid, with warnings",
    "validation.outcome.rejected": "Rejected",

    // The override RPC's own codes.
    "error.validation_not_found": "This import has not been validated, or it belongs to another organisation.",
    "error.override_reason_too_short": "Write at least ten characters explaining the override.",
    "error.override_requires_approver_role": "Only an owner, admin or approver may override a validation warning.",
    "error.already_overridden": "This import was already overridden.",
    "error.override_only_over_warnings": "A file with errors cannot be overridden — only one with warnings can.",

    // add-dedup's own codes.
    "error.already_imported": "This file has already been imported.",
    "error.transfer_not_found": "This transfer no longer exists, or belongs to another organisation.",
    "dedup.level.D2": "Duplicate within this file",
    "dedup.level.D3": "Already imported previously",
    "dedup.transfer.active": "Excluded as a transfer between your own accounts",
    "dedup.transfer.dismissed": "Included",

    "review.title": "Review",
    "review.remaining": "remaining",
    "review.suggested": "Suggested",
    "review.legend": "Keys",
    "review.key.digit": "pick a category",
    "review.key.enter": "approve this counterparty",
    "review.key.transfer": "internal transfer",
    "review.key.notPnl": "not in the P&L",
    "review.key.move": "next · previous",
    "review.key.clear": "clear",
    "review.selected": "Selected",
    "review.transactions": "Transactions",
    "review.groupOf": "of",
    "review.blurb":
      "Classify this counterparty once and every transaction from them -- past and future -- is classified the same way.",
    "review.search.placeholder": "Filter categories",
    "review.search.empty": "No category matches",
    "review.action.approve": "Approve",
    "review.action.transfer": "Mark as transfer",
    "review.action.notPnl": "Not in P&L",

    "report.title": "Management P&L",
    "report.tab.table": "Table",
    "report.tab.charts": "Charts",
    "report.category": "Category",
    "report.total": "Total",
    "report.percentOfRevenue": "% of revenue",
    "report.net": "Net result",
    "report.basis.cash": "Cash-basis",
    "report.basis.accrual": "Accrual",
    "report.basis.note": "derived from the source of the data, not chosen",
    "report.period.from": "From",
    "report.period.to": "To",
    "report.blocked": "Blocked",
    "report.blocked.mixed_basis":
      "This entity has both bank and ledger imports, with no confirmed match between them. Computing a report would count an invoice and its payment twice, so none is shown until they are reconciled.",
    "report.bucket.unclassified": "Unclassified",
    "report.bucket.non_pnl": "Excluded, non-P&L",
    "report.bucket.unallocated": "Unallocated",
    "report.bucket.other_basis": "Other basis",
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
    "drilldown.operands": "Made up of",
    "drilldown.empty": "No transactions in this cell for this period.",
    "drilldown.provenance": "Taxonomy {taxonomy} · ruleset {ruleset} · engine {engine}",
  },
  ru: {
    "app.title": "Veekst",
    "app.tagline": "Управленческая отчётность для собственников бизнеса.",
    "a11y.skipToContent": "Перейти к содержимому",

    "notFound.title": "Страница не найдена",
    "notFound.detail": "По этому адресу ничего нет.",
    "notFound.cta": "Вернуться в Veekst",

    "signIn.heading": "Вход",
    "signIn.google": "Войти через Google",
    "signIn.blurb": "Вход выполняется через аккаунт Google.",

    "signedIn.greeting": "Вы вошли",
    "signedIn.signOut": "Выйти",

    "error.auth_not_configured": "Вход не настроен на этом сервере.",
    "error.invalid_flow": "Ссылка для входа устарела или уже использована. Попробуйте ещё раз.",
    "error.invalid_token": "Не удалось проверить ответ Google. Попробуйте ещё раз.",
    "error.unverified_email": "Адрес электронной почты вашего аккаунта Google не подтверждён.",
    "error.email_taken": "Этот адрес электронной почты уже принадлежит другому аккаунту.",
    "error.internal_error": "Что-то пошло не так. Попробуйте ещё раз.",
    "error.unknown": "Не удалось войти. Попробуйте ещё раз.",

    "error.org_name_required": "Введите название организации.",
    "error.org_entity_name_required": "Введите название юридического лица.",
    "error.org_unsupported_country": "Эта страна пока не поддерживается.",
    "error.org_unsupported_currency": "Эта валюта пока не поддерживается.",
    "error.org_already_a_member": "Вы уже состоите в организации.",

    "firstRun.heading": "Настройте организацию",
    "firstRun.blurb":
      "Одна организация, одно юридическое лицо — для начала. Позже можно пригласить других и добавить больше.",
    "firstRun.name": "Название организации",
    "firstRun.entityName": "Название юридического лица",
    "firstRun.market": "Страна и валюта",
    "firstRun.market.by": "Беларусь — BYN",
    "firstRun.market.kz": "Казахстан — KZT",
    "firstRun.market.pl": "Польша — PLN",
    "firstRun.submit": "Создать организацию",
    "firstRun.submitting": "Создание…",

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

    "nav.primary": "Основное",
    "nav.home": "Главная",
    "nav.imports": "Импорт",
    "nav.review": "Проверка",
    "nav.reports": "Отчёты",
    "nav.awaitingReview": "на проверке",
    "topbar.organisation": "Организация",
    "topbar.entity": "Юр. лицо",
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
    "chart.expenses.title": "Основные статьи расходов",
    "chart.moneyflow.title": "Движение денег",
    "chart.moneyflow.totalIn": "Всего поступило",
    "chart.moneyflow.other": "Прочее",
    "chart.netresult.title": "Итог по месяцам",
    "chart.netresult.series": "Итог",
    "chart.revenueExpense.title": "Выручка и расходы",
    "chart.revenueExpense.revenue": "Выручка",
    "chart.revenueExpense.expenses": "Расходы",
    "chart.trend.title": "Динамика по категориям",
    "chart.view.chart": "График",
    "chart.view.table": "Таблица",

    "landing.cta": "Войти",
    "landing.cta.secondary": "Смотреть отчёт",
    "landing.nav.how": "Как это работает",
    "landing.nav.product": "Отчёт",

    "landing.promise": "Узнайте, сколько бизнес заработал на самом деле.",
    "landing.sub":
      "Veekst превращает банковские выписки и учётные данные бухгалтера в управленческий отчёт, который можно раскрыть построчно — вплоть до операции.",
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

    "home.title": "Главная",
    "home.imports.count": "Импортировано",
    "home.imports.latest": "Последний",
    "home.review.awaiting": "На проверке",
    "home.reports.net": "Финансовый результат",
    "home.account.title": "Аккаунт",

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
    "batch.rows.title": "Строки",
    "batch.rows.category": "Категория",
    "batch.rows.unclassified": "Не классифицировано",
    "error.amount_unparseable": "Сумму не удалось прочитать как число.",
    "error.date_implausible": "Такой даты не существует.",
    "error.currency_unknown": "Недопустимый код валюты ISO-4217.",
    "error.debit_and_credit_both_set": "Заполнены и дебет, и кредит.",
    "error.description_missing": "Описание пустое.",

    "error.upload_missing": "Файл не был получен. Пожалуйста, загрузите его снова.",
    "error.file_too_large": "Файл превышает ограничение в 25 МБ.",
    "error.object_store_not_configured": "Загрузка файлов не настроена на этом сервере.",
    "error.invalid_argument": "Некорректный запрос.",
    "error.batch_not_found": "Этот импорт больше не существует или принадлежит другой организации.",
    "error.not_a_member": "У вас нет доступа к этой организации.",

    "error.replacement_character": "Файл содержит повреждённый текст. Экспортируйте его заново и повторите попытку.",
    "error.account_unresolved": "Не удалось сопоставить счёт из этого файла с уже имеющимся.",
    "error.balance_mismatch": "Входящий остаток плюс обороты не равны заявленному исходящему остатку.",
    "error.row_count_mismatch": "Количество строк не совпадает с заявленным в файле.",
    "error.period_gap": "Есть строка вне периода, заявленного в файле.",
    "error.mixed_currency": "Этот счёт уже числится в другой валюте.",
    "error.duplicate_bank_reference": "Одна и та же проводка встречается в файле дважды.",

    "validation.outcome.valid": "Корректно",
    "validation.outcome.valid_with_warnings": "Корректно, с замечаниями",
    "validation.outcome.rejected": "Отклонено",

    "error.validation_not_found": "Этот импорт ещё не проверен либо принадлежит другой организации.",
    "error.override_reason_too_short": "Укажите не менее десяти символов, поясняющих решение об исключении.",
    "error.override_requires_approver_role": "Исключить предупреждение может только владелец, администратор или утверждающий.",
    "error.already_overridden": "Это решение об исключении уже было принято ранее.",
    "error.override_only_over_warnings": "Файл с ошибками нельзя исключить из проверки — только файл с замечаниями.",

    // add-dedup's own codes. Russian wording has not been checked by a
    // native speaker.
    "error.already_imported": "Этот файл уже был импортирован.",
    "error.transfer_not_found": "Этот перевод больше не существует либо принадлежит другой организации.",
    "dedup.level.D2": "Дублируется внутри этого файла",
    "dedup.level.D3": "Уже был импортирован ранее",
    "dedup.transfer.active": "Исключено как перевод между вашими счетами",
    "dedup.transfer.dismissed": "Учтено",

    "review.title": "Проверка",
    "review.remaining": "осталось",
    "review.suggested": "Предложено",
    "review.legend": "Клавиши",
    "review.key.digit": "выбрать категорию",
    "review.key.enter": "утвердить контрагента",
    "review.key.transfer": "внутренний перевод",
    "review.key.notPnl": "не в ОПиУ",
    "review.key.move": "следующий · предыдущий",
    "review.key.clear": "сбросить",
    "review.selected": "Выбрано",
    "review.transactions": "Операции",
    "review.groupOf": "из",
    "review.blurb":
      "Классифицируйте контрагента один раз — и каждая операция от него, прошлая и будущая, получит ту же категорию.",
    "review.search.placeholder": "Фильтр категорий",
    "review.search.empty": "Категории не найдены",
    "review.action.approve": "Утвердить",
    "review.action.transfer": "Отметить как перевод",
    "review.action.notPnl": "Не в ОПиУ",

    "report.title": "Управленческий ОПиУ",
    "report.tab.table": "Таблица",
    "report.tab.charts": "Графики",
    "report.category": "Категория",
    "report.total": "Итого",
    "report.percentOfRevenue": "% от выручки",
    "report.net": "Финансовый результат",
    "report.basis.cash": "Кассовый метод",
    "report.basis.accrual": "Метод начисления",
    "report.basis.note": "определяется источником данных, а не выбором",
    "report.period.from": "С",
    "report.period.to": "По",
    "report.blocked": "Заблокировано",
    "report.blocked.mixed_basis":
      "У этого юридического лица есть и банковские, и учётные импорты без подтверждённого сопоставления между ними. Расчёт отчёта задвоил бы счёт и его оплату, поэтому отчёт не показывается, пока они не будут сверены.",
    "report.bucket.unclassified": "Не классифицировано",
    "report.bucket.non_pnl": "Исключено, вне P&L",
    "report.bucket.unallocated": "Не распределено",
    "report.bucket.other_basis": "Другой базис",
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
    "drilldown.operands": "Складывается из",
    "drilldown.empty": "В этой ячейке нет операций за этот период.",
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
