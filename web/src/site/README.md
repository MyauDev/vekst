# `site/` — Register A, the public surface

The landing page and sign-in. No session required, and none read.

**This directory must not import from `../data/`, a transport, or the session.**
`add-web-experience` §5.8 tests it. The reason is not tidiness: `ARCHITECTURE.md`
§8 schedules a static marketing bundle at `/site` for the Commercial milestone,
and a page that reaches into application state cannot be lifted into one without
being rewritten. Keeping that move cheap costs nothing today and everything
later.

**Register A scales** (`docs/DESIGN.md` §1): type up to 48px, sections at
64–96px. This is the one directory exempt from the 32px spacing cap in
`scripts/check-web-tokens.sh` — and exempt from that rule only. The colour rule
applies here exactly as it does everywhere else, because both registers share
one token layer.
