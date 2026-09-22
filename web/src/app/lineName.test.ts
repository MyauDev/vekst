import { describe, expect, it } from "vitest";

import { t } from "../i18n";
import { cellName, lineAbbreviation, lineName } from "./lineName";

/**
 * The backend sends the taxonomy's abbreviation as a line's label, on purpose
 * -- a section is keyed by its code and renaming it would move a figure. What
 * that leaves on screen is "OCS", "OIE", "IBT", which is not a name an owner
 * can act on. These assert the translation and, just as importantly, that it
 * fails open.
 */
describe("what a P&L line is called", () => {
  it("resolves the name from the code, not from the label the wire sent", () => {
    // The wire's label for '05' is "OIE" (pnl.go's `sectionLabels`). If this
    // read the label, it would return "OIE".
    expect(lineName("05", "OIE", "en")).toBe(t("line.05", "en"));
    expect(lineName("05", "OIE", "en")).not.toBe("OIE");
    expect(lineName("05", "OIE", "ru")).toBe(t("line.05", "ru"));
  });

  it("names every code the report can print", () => {
    // `report.Order` plus `report.NonPNLSections` -- the sections, the five
    // computed lines, and the two that receive transactions and reach no line.
    const codes = ["01", "02", "03", "04", "05", "06", "07", "08", "09",
      "91", "92", "93", "94", "95"];
    for (const code of codes) {
      for (const locale of ["en", "ru"] as const) {
        const name = lineName(code, "X", locale);
        expect(name, `${code} in ${locale}`).toBeTruthy();
        expect(name, `${code} in ${locale} fell through to the wire's label`).not.toBe("X");
      }
    }
  });

  it("falls back to the wire's own label for a code this build has never heard of", () => {
    // A taxonomy that grows a section renders as whatever the backend called
    // it. Worse than a translation, much better than blank -- the same call
    // `authErrorMessage` makes for an unrecognised error code.
    expect(lineName("10", "SOMETHING NEW", "ru")).toBe("SOMETHING NEW");
  });

  it("keeps an abbreviation worth keeping and drops one that would repeat the name", () => {
    // "IBT" is the anchor an accountant reads down the column for, and it is
    // not reconstructable from "Profit before tax".
    expect(lineAbbreviation("94", "IBT", "en")).toBe("IBT");

    // These two say one thing twice. '01' is the case that actually occurs:
    // the wire's label for it is "NET SALES", which is the name in capitals
    // rather than a code -- "Net sales NET SALES" in one cell.
    expect(lineAbbreviation("01", "NET SALES", "en")).toBe("");
    expect(lineAbbreviation("10", "SOMETHING NEW", "en")).toBe("");

    // Nothing is dropped for a locale whose name does not resemble it.
    expect(lineAbbreviation("01", "NET SALES", "ru")).toBe("NET SALES");
  });

  it("answers a bucket's kind as well as a line's code", () => {
    // `ListLineTransactionsRequest.line` takes two closed, disjoint
    // vocabularies, and the drill-down panel is opened by URL -- it arrives
    // with one of them and no idea which. A bucket has no label on the wire
    // to fall back to, so getting this wrong prints "unclassified" as a
    // heading, which is what it used to do.
    expect(cellName("unclassified", "unclassified", "ru")).toBe(
      t("report.bucket.unclassified", "ru"),
    );
    expect(cellName("other_basis", "other_basis", "en")).toBe(t("report.bucket.other_basis", "en"));
    expect(cellName("91", "GM", "en")).toBe(t("line.91", "en"));
  });
});
