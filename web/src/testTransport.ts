import { Code, ConnectError, createRouterTransport } from "@connectrpc/connect";
import type { ConnectRouter } from "@connectrpc/connect";

import { HealthService } from "./gen/vekst/v1/health_pb";
import { IdentityService } from "./gen/vekst/v1/identity_pb";
import type { Organisation } from "./gen/vekst/v1/identity_pb";

/**
 * A transport that answers from memory. No network, no core, no classifier.
 *
 * `user` null means nobody is signed in, which core reports with the
 * unauthenticated code rather than an empty response -- so the stub has to
 * report it the same way, or the screens under test would be exercising a
 * state the real backend never produces.
 *
 * `organisations` defaults to `[signedInOrganisation]` whenever a user is
 * given, because an empty list is the first-run signal (change 5.3) and most
 * tests using this stub want the ordinary signed-in shell, not the first-run
 * screen. A test exercising first-run itself passes `organisations: []`
 * explicitly.
 *
 * `extend` registers additional services on the same router -- Import,
 * Review, Report -- so a screen test can stub the one RPC it needs without
 * rebuilding Health and Identity itself. `dedup.test.ts` stubs a service
 * directly for a data-module test; this is the same idea one level up, for a
 * test that renders a screen.
 */
export function stubTransport({
  classifierVersion = "engine-1",
  version = "abc1234",
  user = null as { id: string; email: string; name: string; locale: string } | null,
  organisations = user ? [signedInOrganisation] : [],
  extend,
}: {
  classifierVersion?: string;
  version?: string;
  user?: { id: string; email: string; name: string; locale: string } | null;
  organisations?: Organisation[];
  extend?: (router: ConnectRouter) => void;
} = {}) {
  return createRouterTransport((router) => {
    router.service(HealthService, {
      check: () => ({
        status: 1,
        version,
        builtAt: "2026-08-23T12:00:00Z",
        classifierVersion,
      }),
    });
    router.service(IdentityService, {
      getCurrentUser: () => {
        if (!user) {
          throw new ConnectError("unauthenticated", Code.Unauthenticated);
        }
        return { user, organisations };
      },
    });
    extend?.(router);
  });
}

export const signedInUser = {
  id: "11111111-1111-1111-1111-111111111111",
  email: "person@example.test",
  name: "A Person",
  locale: "en",
};

/** One organisation, one entity -- the Demo's own shape. */
export const signedInOrganisation = {
  id: "22222222-2222-2222-2222-222222222222",
  name: "Test Org AS",
  baseCurrency: "NOK",
  role: "owner",
  entities: [{ id: "33333333-3333-3333-3333-333333333333", name: "Test AS" }],
} as Organisation;
