import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { TokenSheet } from "./TokenSheet";
import { t } from "../i18n";

describe("token sheet", () => {
  it("renders all nine states with their words, not colour alone", () => {
    render(<TokenSheet />);

    // docs/DESIGN.md §7: a state always carries a word. If a chip ever renders
    // as a bare swatch this fails, which is the point.
    for (const key of [
      "state.batch.pending",
      "state.batch.parsing",
      "state.batch.rejected",
      "state.batch.imported",
      "state.report.ready",
      "state.report.partial",
      "state.row.classified",
      "state.row.needsReview",
    ] as const) {
      expect(screen.getAllByText(t(key)).length).toBeGreaterThan(0);
    }
    // "Blocked" is two states on two axes and shares one English word.
    expect(screen.getAllByText("Blocked").length).toBe(2);
  });

  it("offers all three theme states", () => {
    render(<TokenSheet />);
    for (const c of ["light", "dark", "system"]) {
      expect(screen.getByRole("button", { name: c })).toBeDefined();
    }
  });
});
