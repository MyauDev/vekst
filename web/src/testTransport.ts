import { Code, ConnectError, createRouterTransport } from "@connectrpc/connect";

import { HealthService } from "./gen/vekst/v1/health_pb";
import { IdentityService } from "./gen/vekst/v1/identity_pb";

/**
 * A transport that answers from memory. No network, no core, no classifier.
 *
 * `user` null means nobody is signed in, which core reports with the
 * unauthenticated code rather than an empty response -- so the stub has to
 * report it the same way, or the screens under test would be exercising a
 * state the real backend never produces.
 */
export function stubTransport({
  classifierVersion = "engine-1",
  version = "abc1234",
  user = null as { id: string; email: string; name: string; locale: string } | null,
} = {}) {
  return createRouterTransport(({ service }) => {
    service(HealthService, {
      check: () => ({
        status: 1,
        version,
        builtAt: "2026-08-23T12:00:00Z",
        classifierVersion,
      }),
    });
    service(IdentityService, {
      getCurrentUser: () => {
        if (!user) {
          throw new ConnectError("unauthenticated", Code.Unauthenticated);
        }
        return { user };
      },
    });
  });
}

export const signedInUser = {
  id: "11111111-1111-1111-1111-111111111111",
  email: "person@example.test",
  name: "A Person",
  locale: "en",
};
