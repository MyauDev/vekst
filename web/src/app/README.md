# `app/` — Register B, the authenticated surface

Everything behind sign-in: the shell, Imports, Review, the report and its
drill-down.

**Register B scales** (`docs/DESIGN.md` §1 and §5): type caps at 24px, the
spacing scale stops at 32px, 32px table rows. `scripts/check-web-tokens.sh`
enforces the spacing cap here. §1.1 is the rule behind it — buy white space by
removing chrome, not by adding padding, because padding lowers the rows per
screen and an accountant comparing twelve periods then scrolls to compare.

Gating is on the route tree, not per screen (`add-web-experience` §2.8): a
screen added here is protected by having been added.
