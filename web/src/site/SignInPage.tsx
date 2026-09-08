/**
 * The sign-in page. Public, and also what `/app` renders to a visitor with no
 * session -- they are already where they meant to be, and bouncing them to
 * another URL loses that.
 */
import { PublicLayout } from "./PublicLayout";
import { SignIn } from "./SignIn";

/** The code the auth routes redirect back with. The backend returns codes and
 *  never sentences, so i18n turns it into something readable. */
function authErrorFromLocation(): string | null {
  if (typeof window === "undefined") return null;
  return new URLSearchParams(window.location.search).get("auth_error");
}

export function SignInPage() {
  return (
    <PublicLayout>
      <div className="max-w-md py-16">
        <SignIn authError={authErrorFromLocation()} />
      </div>
    </PublicLayout>
  );
}
