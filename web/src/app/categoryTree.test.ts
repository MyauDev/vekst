import { describe, expect, it } from "vitest";

import { sideOf } from "./categoryTree";

describe("which side of the P&L a category is on", () => {
  it("reads it from the section, and one level down where a section is mixed", () => {
    expect(sideOf("0101")).toBe("income");
    expect(sideOf("050202")).toBe("income");
    expect(sideOf("060201")).toBe("income");
    expect(sideOf("0405010102")).toBe("expense");
    expect(sideOf("05010305")).toBe("expense");
    expect(sideOf("07")).toBe("expense");
  });

  it("offers on both sides what can go either way, or is unknown", () => {
    expect(sideOf("060101")).toBe("both");
    expect(sideOf("09")).toBe("both");
    expect(sideOf("9901")).toBe("both");
  });
});
