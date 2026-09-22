import { beforeEach, describe, expect, it } from "vitest";

import { getPeriodRange, initPeriodRange, setPeriodRange } from "./periodPreference";
import { defaultRange } from "./period";

describe("period preference", () => {
  beforeEach(() => {
    localStorage.clear();
    initPeriodRange();
  });

  it("falls back to year-to-date the first time this browser has ever opened a report", () => {
    expect(getPeriodRange()).toEqual(defaultRange());
  });

  it("survives a reload", () => {
    setPeriodRange({ from: "2023-01", to: "2024-12" });
    initPeriodRange();
    expect(getPeriodRange()).toEqual({ from: "2023-01", to: "2024-12" });
  });

  it("is a stored preference, not a URL parameter", () => {
    setPeriodRange({ from: "2023-01", to: "2024-12" });
    expect(window.location.search).toBe("");
  });

  it("ignores a corrupted stored value rather than crash", () => {
    localStorage.setItem("veekst.period.from", "not-a-period");
    localStorage.setItem("veekst.period.to", "2024-12");
    initPeriodRange();
    expect(getPeriodRange()).toEqual(defaultRange());
  });
});
