> **RETROACTIVE.** A ticked box below means the code exists at `origin/main` b4d8dca and
> says which file or test satisfies it. An open box is genuinely missing work.

**Budget.** `docs/IMPLEMENTATION_PLAN.md` §3 allocates **3 person-days** to change 2.2.
What landed is roughly 2 of them; the open items in §4 and §5 are the remaining 1, and §4
cannot start until change 2.1 provides a batch to hang raw rows off.

**Ordering.** §1–§3 are done. §4 is blocked by 2.1. §5 is blocked by nothing and should be
done first, because two other changes are being written against the wording it corrects.

**Ownership.** Track A.

## 1. The parser contract — done

- [x] 1.1 `Parser` interface: `Name`, `Detect`, `Parse`. One parser per bank, pure over bytes — `core/internal/ingest/parser.go`
- [x] 1.2 Registry populated from each parser's `init`, sorted by name, with `Formats()` — `parser.go`
- [x] 1.3 `ParserFor` refuses on ambiguity and names every parser that claimed the file — `parser.go`; `TestDetectionDoesNotClaimForeignFiles`
- [x] 1.4 `ErrUnknownFormat` carries the list of formats tried — `parser.go`; `TestUnknownFormatNamesWhatWasTried`
- [x] 1.5 `Parse` stamps `Statement.Format` with the parser that read the file — `parser.go`; `TestParseDetectsTheFormat`

## 2. Charset and number locale — done

- [x] 2.1 `DetectCharset` — UTF-8, windows-1251, CP866, by Cyrillic-versus-box-drawing score over an 8 KiB sample — `decode.go`
- [x] 2.2 `Decode`, stripping a BOM, returning U+FFFD rather than failing — `decode.go`
- [x] 2.3 `ContainsReplacementChar`, for validation to call — `decode.go`
- [x] 2.4 `DecimalFromLocale` resolves the decimal separator by position and hands a decimal **string** to `money.Parse`; no `float64` anywhere on the path — `locale.go`
- [x] 2.5 Date-format handling kept out of the parser: `Row.BookedOn` stays raw — `priorbank.go`

## 3. The Priorbank reader — done

- [x] 3.1 `ParsePriorbank`, preamble, header location by name, both column layouts — `priorbank.go`; `TestBothColumnLayoutsParse`
- [x] 3.2 `Detect` requires the bank name **and** the `Дата док` header token within 4 KiB — `priorbank.go`; `TestNotAPriorbankExportIsRejected`
- [x] 3.3 Summary rows read positionally, not by header index — `priorbank.go` §`summaryPair`, `summaryBalance`
- [x] 3.4 Statement metadata: account, currency, holder, opening, closing, declared turnover — `TestStatementMetadataIsRead`
- [x] 3.5 Every row carries `LineNo`, the 1-based line in the **original** file — `TestLineNumbersReferToTheOriginalFile`
- [x] 3.6 `Statement.BalanceCheck()` — zero tolerance, in minor units, returning the difference — `TestBalanceReconcilesOnEveryFixture`
- [x] 3.7 Declared turnover reconciles against the parsed rows — `TestDeclaredTurnoverMatchesParsedRows`
- [x] 3.8 `RegulatedCode` on `Row`, empty for Belarus, carried ahead of need for КНП and 1C
- [x] 3.9 Four redacted real exports in `core/testdata/priorbank-by`, covering both layouts

## 4. Raw rows — open, blocked by 2.1

- [ ] 4.1 Persist `raw_rows(id, org_id, batch_id, line_no, payload_jsonb)` per `ARCHITECTURE.md` §5.5. Needs a batch to point at, so it lands with or after change 2.1's migration 008
- [ ] 4.2 Cross-tenant isolation test for `raw_rows`, and `FORCE ROW LEVEL SECURITY` like every tenant table
- [ ] 4.3 Decide whether the whole decoded text is retained or only the parsed cells. The object store already keeps the original file, so this is about how a parse is re-examined, not about durability

## 5. Correct the planning documents — open, blocked by nothing

- [ ] 5.1 Correct `docs/ARCHITECTURE.md` §4a.1: the debit/credit check fails when both are **non-zero**, not when both are populated. As written it rejects every row of every real file (design D5)
- [ ] 5.2 Correct `docs/IMPLEMENTATION_PLAN.md` §1.1 the same way, and tell whoever is writing change 2.3 — its spec was drafted from the wrong wording
- [ ] 5.3 Record in `ARCHITECTURE.md` §4a.1 that the balance check is **measured** by `Statement.BalanceCheck()` and **decided** by change 2.3, so it does not get implemented twice (design D6)
- [ ] 5.4 Note in `docs/IMPLEMENTATION_PLAN.md` §3 that 2.2 shipped inside PR #4 without a change, and that this document is retroactive

## 6. Second format — open, not Demo scope

- [ ] 6.1 A second parser, to prove the registry claim rather than assert it. Kazakh and Polish statements exist in `../docCl` and only need the redaction treatment the Priorbank fixtures already had
- [ ] 6.2 XLSX input. `IMPLEMENTATION_PLAN.md` §3 lists it under 2.2; nothing reads a workbook yet

## 7. Close

- [ ] 7.1 Add `/core/internal/ingest/` to `.github/CODEOWNERS` under Track A — the directory exists and CODEOWNERS names it already; confirm rather than assume
- [ ] 7.2 Update the capability spec and run the full suite
