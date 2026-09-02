import { create } from "@bufbuild/protobuf";
import { describe, expect, it } from "vitest";

import { MoneySchema } from "./gen/vekst/type/v1/money_pb";

// Task 5.7 / ARCHITECTURE.md §6: money crosses the wire as a string, never a
// JS number, which cannot hold an int64 exactly. Asserts the outcome, not
// the mechanism (jstype = JS_STRING on the .proto field) -- so this still
// catches a regression even if a future generator version stops honouring
// that option, which design's own risk table flags as a real possibility.
describe("Money", () => {
  it("carries minor_units as a string, never a number", () => {
    const money = create(MoneySchema, { currencyCode: "EUR", minorUnits: "1234" });

    expect(typeof money.minorUnits).toBe("string");
    expect(money.minorUnits).toBe("1234");
  });

  it("round-trips an amount too large for a JS number to hold exactly", () => {
    // Number.MAX_SAFE_INTEGER is 2^53-1; this is well past it and would
    // silently lose precision if minor_units were ever a JS number.
    const large = "9007199254740993";
    const money = create(MoneySchema, { currencyCode: "JPY", minorUnits: large });

    expect(money.minorUnits).toBe(large);
  });
});
