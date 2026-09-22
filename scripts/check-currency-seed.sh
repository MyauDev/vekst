#!/usr/bin/env bash
# The migration's copy of `currencies` agrees with the generator that produced
# it, row for row.
#
# Unlike the taxonomy (check-taxonomy-seed.sh), core/internal/money's exponent
# map has no source outside this repository, so this check can do what that
# one cannot: run the generator live and diff its output directly against the
# migration, rather than comparing two checked-in copies. A drift here means
# someone hand-edited the migration's INSERT block after generating it, or
# edited core/internal/money/exponents.go and forgot to re-copy the rows.
#
# Run from the repository root, or through `make lint`.

set -euo pipefail

migration="core/migrations/00018_category_templates_and_currencies.sql"

if [[ ! -f $migration ]]; then
  echo "check-currency-seed: $migration is missing" >&2
  exit 1
fi

# The seeded rows, as they appear in the migration: "  ('AED', 2)," .
rows_in_migration() { grep -E "^  \('[A-Z]{3}', [0-9]+\)" "$migration" || true; }

generated=$(go run ./core/internal/money/cmd/gen-currency-seed)
embedded=$(rows_in_migration)

if [[ -z $embedded ]]; then
  echo "check-currency-seed: no currency rows found in $migration --" >&2
  echo "  the migration's row format changed and this check stopped checking." >&2
  exit 1
fi

if ! diff <(echo "$generated") <(echo "$embedded") >/dev/null; then
  echo "check-currency-seed: $migration no longer matches" >&2
  echo "  'go run ./core/internal/money/cmd/gen-currency-seed'." >&2
  echo "  Re-run it and copy the INSERT block into the migration." >&2
  diff <(echo "$generated") <(echo "$embedded") | head -20 >&2
  exit 1
fi

echo "check-currency-seed: $(echo "$embedded" | wc -l | tr -d ' ') currencies, migration matches the generator"
