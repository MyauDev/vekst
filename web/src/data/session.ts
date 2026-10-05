/**
 * The organisation on screen, held in one place.
 *
 * `add-web-experience` design D1 requires that connecting a real backend
 * replaces a module body and touches no component, and `data.test.ts` holds a
 * test to it: no screen imports a transport. Giving `imports.ts`, `report.ts`
 * and `review.ts` an organisation argument would push that identifier through
 * every screen and break both. So it is ambient instead: `AppLayout` sets it
 * once, after `GetCurrentUser` resolves and before it renders any screen, and
 * the data modules read it back through `requireSession()`.
 *
 * A data call before the session is set is a programming error, and
 * `requireSession()` throws rather than sending an empty `organization_id` --
 * a wrong error in front of a right one is the harder bug, because the
 * server's answer (`not_a_member` for an empty id) reads like a real
 * refusal instead of a wiring mistake.
 */

export interface Session {
  orgId: string;
  entityId: string;
  baseCurrency: string;
}

let current: Session | undefined;

export function setSession(s: Session): void {
  current = s;
}

export function clearSession(): void {
  current = undefined;
}

export class SessionNotSetError extends Error {
  constructor() {
    super("session: requireSession() called before setSession()");
    this.name = "SessionNotSetError";
  }
}

export function requireSession(): Session {
  if (!current) throw new SessionNotSetError();
  return current;
}
