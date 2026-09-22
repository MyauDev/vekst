/**
 * jsdom implements no ResizeObserver. `@floating-ui/react` -- what
 * `react-datepicker` (PeriodPicker.tsx) positions its popup with -- creates
 * one on mount to keep the popup anchored as its target resizes; with no
 * global to construct, the popup never opens in a test, which reads as a
 * hang (`findByText` polling something that will never render) rather than a
 * clear failure. The real behaviour it tracks needs a laid-out DOM jsdom does
 * not do either, so a no-op is what a test can honestly assert against.
 *
 * Not installed globally in `test-setup.ts`: echarts/zrender (the Sankey
 * chart in `MoneyFlowChart`) also feature-detects `ResizeObserver`, but
 * expects the real thing -- once one exists it stops using its own
 * synchronous fallback and waits on a callback this no-op never fires, which
 * crashed every chart-tab test in `report.test.tsx` the one time this was
 * tried as a global. `vi.stubGlobal("ResizeObserver", NoopResizeObserver)` in
 * the specific test that opens a picker, paired with `vi.unstubAllGlobals()`
 * in that file's `afterEach`, keeps the two chart libraries from ever seeing
 * each other's assumptions about what the global means.
 */
export class NoopResizeObserver {
  observe() {}
  unobserve() {}
  disconnect() {}
}
