#!/usr/bin/env bash
# Fails if a component writes a colour instead of naming one, or if Register B
# spends more than 32px on a single gap.
#
# docs/DESIGN.md §3 prices dark mode at half a day. That price holds only while
# every component uses a semantic token name: `text-slate-900` in a component
# turns a second block of values into a rewrite. The document has called such a
# class "a defect" since 2026-09-06 and nothing enforced it, so there were 22 of
# them by the time anyone counted. This is the enforcement -- the same move as
# check-db-entry-point.sh and check-identity-queries.sh, which turn a rule in a
# document into a failing build.
#
# The token layer is web/src/index.css. It is the one file allowed to hold a
# colour literal, because it is the file whose job is holding them.
#
# Register A (web/src/site, the landing page) is exempt from the spacing rule
# only. DESIGN.md §1 gives it sections at 64-96px. It is NOT exempt from the
# colour rule: both registers share one token layer.
set -euo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")/.."

SRC="web/src"
TOKENS="$SRC/index.css"

if [ ! -f "$TOKENS" ]; then
  echo "::error::$TOKENS is missing; this check has nothing to enforce"
  exit 1
fi

# Generated clients are not ours. The mock is scheduled for deletion with 5.2
# and holds its own isolated token layer on purpose (web/src/mock/README.md).
sources() {
  find "$SRC" -type f \( -name '*.ts' -o -name '*.tsx' -o -name '*.css' \) \
    -not -path "$SRC/gen/*" \
    -not -path "$SRC/mock/*" \
    -not -path "$TOKENS" \
    | sort
}

status=0
files=$(sources)
[ -z "$files" ] && { echo "no source files to check"; exit 0; }

PALETTE='slate|gray|zinc|neutral|stone|red|orange|amber|yellow|lime|green|emerald|teal|cyan|sky|blue|indigo|violet|purple|fuchsia|pink|rose'
PROPS='text|bg|border|ring|fill|stroke|divide|outline|decoration|shadow|accent|caret|from|via|to|placeholder'

# 1. A named colour from Tailwind's own palette. The whole defect, by name.
if hits=$(grep -nEH "\\b(${PROPS})-(${PALETTE})-[0-9]{2,3}\\b" $files 2>/dev/null); then
  echo "::error::literal Tailwind colour classes -- use a semantic token from ${TOKENS}:"
  echo "$hits"
  status=1
fi

# 2. black and white. Tempting in a monochrome design and wrong for the same
#    reason: neither follows the theme, so both survive into dark mode intact.
if hits=$(grep -nEH "\\b(${PROPS})-(black|white)\\b" $files 2>/dev/null); then
  echo "::error::'black'/'white' do not follow the theme -- use text/surface tokens:"
  echo "$hits"
  status=1
fi

# 3. A raw colour value. Same defect wearing a different hat.
if hits=$(grep -nEH '#[0-9a-fA-F]{3,8}\b|\b(rgba?|hsla?|oklch|oklab|color-mix)\(' $files 2>/dev/null); then
  echo "::error::raw colour values belong in ${TOKENS}, not in components:"
  echo "$hits"
  status=1
fi

# 4. Spacing above 32px inside Register B.
#
#    DESIGN.md §5 fixes the app's scale at 2, 4, 6, 8, 12, 16, 24, 32. With
#    --spacing: 0.25rem those are 0.5 1 1.5 2 3 4 6 8, so any step above 8 is
#    over budget. Tailwind v4 generates spacing utilities dynamically -- `p-96`
#    compiles -- so this cannot be enforced by the token layer and has to live
#    here.
#
#    §1.1 is the reason: a minimal design that buys white space with padding
#    lowers the rows per screen, "and an accountant who compares twelve periods
#    then scrolls to compare."
GAPS='p|px|py|pt|pr|pb|pl|ps|pe|m|mx|my|mt|mr|mb|ml|ms|me|gap|gap-x|gap-y|space-x|space-y'
register_b=$(printf '%s\n' $files | grep -v "^$SRC/site/" || true)

if [ -n "$register_b" ]; then
  over=$(grep -noEH "\\b-?(${GAPS})-[0-9]+(\\.[0-9]+)?\\b" $register_b 2>/dev/null \
    | awk -F: '{
        cls = $NF
        n = cls; sub(/^-/, "", n); sub(/^[a-z-]+-/, "", n)
        if (n + 0 > 8) print $1 ":" $2 ": " cls
      }' || true)
  if [ -n "$over" ]; then
    echo "::error::spacing above 32px in the application -- DESIGN.md §5 caps the app scale at 32 (step 8):"
    echo "$over"
    status=1
  fi

  # Arbitrary spacing sidesteps the scale entirely.
  if hits=$(grep -nEH "\\b-?(${GAPS})-\\[" $register_b 2>/dev/null); then
    echo "::error::arbitrary spacing values bypass the scale -- use a step from DESIGN.md §5:"
    echo "$hits"
    status=1
  fi
fi

if [ $status -eq 0 ]; then
  echo "web tokens: ok ($(printf '%s\n' $files | wc -l | tr -d ' ') files)"
fi

exit $status
