import { clearSession } from "./session";

/**
 * Sign-out. The one write in this directory that is not a Connect RPC.
 *
 * A plain fetch, not an RPC: sign-out clears a cookie in the same response
 * that ends the session, and it belongs with the other two auth routes
 * (`CLAUDE.md`: `/auth/google/start`, `/auth/google/callback` and
 * `/auth/logout` are the one non-Connect browser surface).
 */
export async function signOut() {
  clearSession();
  await fetch("/auth/logout", { method: "POST" });
  window.location.assign("/");
}
