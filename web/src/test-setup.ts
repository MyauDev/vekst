/**
 * Testing Library's `findBy*` waits 1000ms by default.
 *
 * The report screen resolves a router, a guard and a query before it renders,
 * and under a parallel run that occasionally exceeds a second — which showed up
 * as a test that passed alone and failed in the suite. An intermittently red
 * test is worse than no test: it teaches people to re-run rather than to look.
 *
 * The assertions were right and the budget was wrong, so the budget moves. This
 * does not slow a passing test down; it only changes how long a failing one
 * waits before admitting it.
 */
import { configure } from "@testing-library/react";

configure({ asyncUtilTimeout: 5000 });
