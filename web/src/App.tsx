import { useQuery } from "@tanstack/react-query";
import { Code, ConnectError, createClient, type Transport } from "@connectrpc/connect";

import { HealthCard } from "./HealthCard";
import { SignIn } from "./SignIn";
import { SignedIn } from "./SignedIn";
import { IdentityService } from "./gen/vekst/v1/identity_pb";
import { t } from "./i18n";

/**
 * Reads the `auth_error` code the auth routes redirect back with. The backend
 * returns codes and never sentences, so this is a code and i18n turns it into
 * something readable.
 */
function authErrorFromLocation(): string | null {
  if (typeof window === "undefined") return null;
  return new URLSearchParams(window.location.search).get("auth_error");
}

async function signOut() {
  // A plain fetch rather than an RPC: sign-out has to clear a cookie in the
  // same response that ends the session, and it belongs with the other two
  // auth routes.
  await fetch("/auth/logout", { method: "POST" });
  window.location.assign("/");
}

export function App({ transport }: { transport: Transport }) {
  return (
    <main className="mx-auto flex min-h-dvh max-w-lg flex-col justify-center gap-6 p-8">
      <div>
        <h1 className="text-2xl font-semibold text-slate-900">{t("app.title")}</h1>
        <p className="text-sm text-slate-500">{t("app.tagline")}</p>
      </div>
      <Session transport={transport} />
    </main>
  );
}

/**
 * Decides between the sign-in screen and the application shell.
 *
 * An unauthenticated call is not an error condition here: it is the ordinary
 * state of a browser that has not signed in yet, and core answers it with the
 * unauthenticated code by design. Anything else is a real failure and says so.
 */
export function Session({ transport }: { transport: Transport }) {
  const { data, error, isPending } = useQuery({
    queryKey: ["currentUser"],
    queryFn: () => createClient(IdentityService, transport).getCurrentUser({}),
    retry: false,
  });

  if (isPending) return <p className="text-slate-500">…</p>;

  if (error) {
    if (ConnectError.from(error).code === Code.Unauthenticated) {
      return <SignIn authError={authErrorFromLocation()} />;
    }
    return <p className="text-red-600">core unreachable: {ConnectError.from(error).message}</p>;
  }

  if (!data.user) return <SignIn authError={authErrorFromLocation()} />;

  return (
    <>
      <SignedIn user={data.user} onSignOut={signOut} />
      <HealthCard transport={transport} />
    </>
  );
}
