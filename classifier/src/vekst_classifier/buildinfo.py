"""Identity of the running build.

Set from the environment at image build time, the same way core is stamped with
-ldflags, so the two services report versions the same way. An unstamped build
reports "dev" rather than an empty string, so a missing stamp is visible instead
of looking like a value nobody set.
"""

import os

_UNSTAMPED = "dev"


def engine_version() -> str:
    """Git SHA of this build, or "dev" when unstamped.

    From change 3.2 this string is recorded on every classification row and
    pinned into every report, alongside taxonomy_version and ruleset_version.
    """
    return os.environ.get("VEKST_VERSION") or _UNSTAMPED


def built_at() -> str:
    """Build time as RFC 3339, or "dev" when unstamped."""
    return os.environ.get("VEKST_BUILT_AT") or _UNSTAMPED
