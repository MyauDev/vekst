/*
 * Mock data for the screens under src/mock.
 *
 * Every amount is int64 minor units as a string, exactly as the wire carries
 * it. Figures are invented, but they are internally consistent: the totals,
 * the subtotals, the percentages and the reconciliation strip all compute from
 * these arrays rather than being typed in.
 */
import type { Locale } from "./money";

export const MONTHS = ["Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug"] as const;

export const ORG = "Nordvest Handel AS";
export const CURRENCY = "EUR";
export const PROVENANCE = "taxonomy v3 · ruleset v11 · engine 0.4.2";

export type Section = "Revenue" | "Cost of sales" | "Operating expenses";

export interface PnlLine {
  section: Section;
  label: string;
  /** One minor-unit string per month. Empty when the line is blocked. */
  values: readonly string[];
  /** Set when the line is refused rather than computed. */
  blockedReason?: string;
}

export const PNL_LINES: readonly PnlLine[] = [
  {
    section: "Revenue",
    label: "Product sales",
    values: ["5240000", "5120000", "5590000", "5460000", "5150000", "5780000", "5540000", "5840000"],
  },
  {
    section: "Revenue",
    label: "Services",
    values: ["1410000", "1520000", "1470000", "1640000", "1560000", "1510000", "1680000", "1740000"],
  },
  {
    section: "Revenue",
    label: "Consulting income",
    values: [],
    blockedReason: "mixed sources, no D4 match",
  },
  {
    section: "Cost of sales",
    label: "Materials",
    values: ["-2270000", "-2210000", "-2440000", "-2340000", "-2240000", "-2550000", "-2390000", "-2560000"],
  },
  {
    section: "Cost of sales",
    label: "Inbound freight",
    values: ["-303000", "-292000", "-336000", "-325000", "-299000", "-347000", "-329000", "-355000"],
  },
  {
    section: "Operating expenses",
    label: "Payroll",
    values: ["-1920000", "-1920000", "-1920000", "-2010000", "-2010000", "-2010000", "-2010000", "-2070000"],
  },
  {
    section: "Operating expenses",
    label: "Logistics",
    values: ["-664020", "-603075", "-681240", "-631860", "-624025", "-710580", "-665535", "-726090"],
  },
  {
    section: "Operating expenses",
    label: "Rent and utilities",
    values: ["-350000", "-350000", "-350000", "-350000", "-350000", "-350000", "-350000", "-350000"],
  },
  {
    section: "Operating expenses",
    label: "Software and subscriptions",
    values: ["-156600", "-156600", "-162150", "-162150", "-162150", "-166600", "-166600", "-166600"],
  },
  {
    section: "Operating expenses",
    label: "Professional fees",
    values: ["-78000", "0", "-125500", "0", "-66500", "0", "-192000", "0"],
  },
];

export const SECTIONS: readonly Section[] = ["Revenue", "Cost of sales", "Operating expenses"];

/** Opening + in + out + transfers = closing. The screen asserts it rather than printing it. */
export const RECONCILIATION = {
  opening: "15230000",
  moneyIn: "57482040",
  moneyOut: "-48611255",
  transfers: "-418000",
} as const;

/* ---------- imports ---------- */

export type BatchState = "pending" | "parsing" | "rejected" | "imported";

export interface Batch {
  id: string;
  file: string;
  sourceKind: "bank" | "ledger";
  period: string;
  rows: number | null;
  state: BatchState;
  note?: string;
  uploaded: string;
  summary?: { imported: number; duplicates: number; transfers: number; matches: number };
}

export const BATCHES: readonly Batch[] = [
  {
    id: "b-08",
    file: "nordea_1234_2026-08.csv",
    sourceKind: "bank",
    period: "01 – 31 Aug 2026",
    rows: 3182,
    state: "rejected",
    uploaded: "06 Sep 09:11",
  },
  {
    id: "b-07",
    file: "nordea_1234_2026-07.csv",
    sourceKind: "bank",
    period: "01 – 31 Jul 2026",
    rows: 3041,
    state: "imported",
    uploaded: "04 Sep 17:42",
    summary: { imported: 2953, duplicates: 88, transfers: 6, matches: 12 },
  },
  {
    id: "l-07",
    file: "1c_postings_2026-07.xlsx",
    sourceKind: "ledger",
    period: "01 – 31 Jul 2026",
    rows: 1884,
    state: "imported",
    uploaded: "04 Sep 17:38",
  },
  {
    id: "b-06",
    file: "nordea_1234_2026-06.csv",
    sourceKind: "bank",
    period: "01 – 30 Jun 2026",
    rows: 2904,
    state: "imported",
    uploaded: "01 Sep 11:02",
  },
  {
    id: "b-06-dup",
    file: "nordea_1234_2026-06 (copy).csv",
    sourceKind: "bank",
    period: "01 – 30 Jun 2026",
    rows: null,
    state: "rejected",
    note: "already imported",
    uploaded: "01 Sep 11:04",
  },
];

export interface ValidationError {
  line: number;
  code: string;
  en: string;
  ru: string;
}

/**
 * The backend returns the code. Every sentence below is the client's — that is
 * the invariant in CLAUDE.md, and the whole of change 5.1b.
 */
export const VALIDATION_ERRORS: readonly ValidationError[] = [
  {
    line: 1204,
    code: "AMOUNT_UNPARSEABLE",
    en: "Amount “1 240,00-” is not a number in this file’s locale",
    ru: "Сумма «1 240,00-» не читается как число в локали этого файла",
  },
  {
    line: 2880,
    code: "CHARSET_REPLACEMENT",
    en: "Row holds replacement characters. The file is probably Windows-1251, not UTF-8",
    ru: "В строке символы замены. Вероятно, файл в Windows-1251, а не в UTF-8",
  },
  {
    line: 3051,
    code: "DEBIT_AND_CREDIT_BOTH_SET",
    en: "Debit and credit are both filled on the same row",
    ru: "В одной строке заполнены и дебет, и кредит",
  },
];

export const BALANCE_CHECK = {
  opening: "41230000",
  movements: "-1844215",
  closing: "39386000",
  difference: "215",
} as const;

/* ---------- review queue ---------- */

export interface ReviewTransaction {
  date: string;
  description: string;
  amount: string;
}

export interface ReviewGroup {
  id: string;
  counterparty: string;
  rows: number;
  amount: string;
  layer: string;
  confidence: string;
  suggested: string;
  transactions: readonly ReviewTransaction[];
}

export const REVIEW_GROUPS: readonly ReviewGroup[] = [
  {
    id: "vektor",
    counterparty: "OOO Vektor Logistics",
    rows: 14,
    amount: "-681240",
    layer: "L2",
    confidence: "0.61",
    suggested: "Logistics",
    transactions: [
      { date: "14 Mar", description: "VEKTOR LOGISTIKA OPLATA SCHET 4471", amount: "-128460" },
      { date: "02 Mar", description: "VEKTOR LOGISTIKA OPLATA SCHET 4390", amount: "-110200" },
      { date: "21 Feb", description: "VEKTOR LOG. VOZVRAT PEREPLATY", amount: "31820" },
    ],
  },
  {
    id: "baltic",
    counterparty: "Baltic Freight Partners",
    rows: 9,
    amount: "-514000",
    layer: "L2",
    confidence: "0.58",
    suggested: "Inbound freight",
    transactions: [
      { date: "07 Mar", description: "BALTIC FREIGHT PARTNERS INV 2026-0331", amount: "-194000" },
      { date: "18 Feb", description: "BALTIC FREIGHT PARTNERS INV 2026-0290", amount: "-166500" },
    ],
  },
  {
    id: "kontorbygg",
    counterparty: "Kontorbygg Drift AS",
    rows: 8,
    amount: "-428000",
    layer: "L1",
    confidence: "0.72",
    suggested: "Rent and utilities",
    transactions: [
      { date: "01 Mar", description: "KONTORBYGG DRIFT AS LEIE MARS", amount: "-175000" },
      { date: "01 Feb", description: "KONTORBYGG DRIFT AS LEIE FEBRUAR", amount: "-175000" },
    ],
  },
  {
    id: "prochie",
    counterparty: "ЗАО «Прочие 4410»",
    rows: 5,
    amount: "-390515",
    layer: "L0.5",
    confidence: "0.44",
    suggested: "",
    transactions: [
      { date: "22 Mar", description: "СЧЁТ 4410 ПРОЧИЕ РАСХОДЫ", amount: "-142300" },
      { date: "11 Feb", description: "СЧЁТ 4410 ПРОЧИЕ РАСХОДЫ", amount: "-118900" },
    ],
  },
  {
    id: "stripe",
    counterparty: "Stripe Payments Europe",
    rows: 4,
    amount: "241080",
    layer: "L2",
    confidence: "0.66",
    suggested: "Services",
    transactions: [
      { date: "31 Mar", description: "STRIPE PAYMENTS EUROPE PAYOUT", amount: "96400" },
      { date: "28 Feb", description: "STRIPE PAYMENTS EUROPE PAYOUT", amount: "88200" },
    ],
  },
];

export const CATEGORIES: readonly string[] = [
  "Logistics",
  "Inbound freight",
  "Materials",
  "Rent and utilities",
  "Professional fees",
  "Software and subscriptions",
  "Services",
  "Product sales",
];

/* ---------- drill-down ---------- */

export interface DrilldownRow {
  date: string;
  description: string;
  amount: string;
  category: string;
  layer: string;
  confidence: string;
  evidence?: string;
  fromVendorMemory?: boolean;
}

/**
 * Only one cell carries a transaction list: Logistics × March.
 * Populating every cell would mean inventing a few hundred transactions, and
 * invented data is what the plan bans. Every other cell opens the panel with
 * its real figure and says the list is not part of the mock.
 */
export const DRILLDOWN_KEY = "Logistics:2";

export const DRILLDOWN_ROWS: readonly DrilldownRow[] = [
  {
    date: "14 Mar",
    description: "VEKTOR LOGISTIKA OPLATA SCHET 4471",
    amount: "-128460",
    category: "Logistics",
    layer: "L2",
    confidence: "0.61",
  },
  {
    date: "07 Mar",
    description: "BALTIC FREIGHT PARTNERS INV 2026-0331",
    amount: "-194000",
    category: "Logistics",
    layer: "L1",
    confidence: "0.94",
    evidence: "D4 matched to ledger document INV-2026-0331 · confirmed 04 Sep",
  },
  {
    date: "28 Mar",
    description: "POSTEN BRING AS FRAKT",
    amount: "-48620",
    category: "Logistics",
    layer: "L0",
    confidence: "1.00",
    fromVendorMemory: true,
  },
  {
    date: "03 Mar",
    description: "DHL EXPRESS NORGE AS",
    amount: "-61235",
    category: "Logistics",
    layer: "L0",
    confidence: "1.00",
    fromVendorMemory: true,
  },
  {
    date: "02 Mar",
    description: "VEKTOR LOGISTIKA OPLATA SCHET 4390",
    amount: "-110200",
    category: "Logistics",
    layer: "L2",
    confidence: "0.61",
  },
];

/* ---------- words ---------- */

type Words = Record<Locale, string>;

/** docs/DESIGN.md §7. The Russian column is a draft and needs a native check. */
export const T: Record<string, Words> = {
  imports: { en: "Imports", ru: "Импорт" },
  review: { en: "Review", ru: "Проверка" },
  reports: { en: "Reports", ru: "Отчёты" },
  pnl: { en: "Management P&L", ru: "Управленческий P&L" },
  reviewQueue: { en: "Review queue", ru: "Очередь проверки" },
  tokens: { en: "Token sheet", ru: "Токены" },
  category: { en: "Category", ru: "Категория" },
  total: { en: "Total", ru: "Итого" },
  pctRev: { en: "% rev", ru: "% выр." },
  totalRevenue: { en: "Total revenue", ru: "Выручка, итого" },
  grossProfit: { en: "Gross profit", ru: "Валовая прибыль" },
  ebitda: { en: "EBITDA", ru: "EBITDA" },
  pending: { en: "Pending", ru: "Ожидает" },
  parsing: { en: "Parsing", ru: "Обработка" },
  rejected: { en: "Rejected", ru: "Отклонён" },
  imported: { en: "Imported", ru: "Импортирован" },
  ready: { en: "Ready", ru: "Готов" },
  partial: { en: "Partial", ru: "Частично" },
  blocked: { en: "Blocked", ru: "Заблокирован" },
  cashBasis: { en: "Cash basis · bank", ru: "Кассовый метод · банк" },
  opening: { en: "Opening balance", ru: "Входящий остаток" },
  moneyIn: { en: "Money in", ru: "Поступления" },
  moneyOut: { en: "Money out", ru: "Списания" },
  transfers: { en: "Internal transfers", ru: "Внутренние переводы" },
  closing: { en: "Closing balance", ru: "Исходящий остаток" },
  balances: { en: "Balances", ru: "Сходится" },
};

export function t(key: keyof typeof T, lang: Locale): string {
  return T[key]?.[lang] ?? key;
}
