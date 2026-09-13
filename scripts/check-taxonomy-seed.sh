#!/usr/bin/env bash
# Three consistency checks over the generated taxonomy, none of which needs the
# founder's source files.
#
# eval/emit.py reads ../docCl, which is deliberately outside the repository: it
# holds a customer's own bank exports and their accountant's categorisation. So
# CI cannot regenerate the output and diff it, the way the codegen job does for
# /proto. What CI can do is check that the committed artefacts still agree with
# each other, and that is where the failure this guards against actually lives:
# a rule pointing at a category that no longer exists, or a migration whose
# copy of the seed has drifted from the generator's.
#
# Run from the repository root, or through `make lint`.

set -euo pipefail

seed_categories="eval/out/seed_categories.sql"
seed_rules="eval/out/seed_rules.sql"
migration="core/migrations/00005_classification_taxonomy.sql"
migration_rules="core/migrations/00006_classification_rules.sql"
status=0

for f in "$seed_categories" "$seed_rules" "$migration" "$migration_rules"; do
  if [[ ! -f $f ]]; then
    echo "check-taxonomy-seed: $f is missing" >&2
    exit 1
  fi
done

# The seeded rows, as they appear in both files: "  ('v1', '0401', ..." .
rows_in() { grep -E "^  \('v1', " "$1" || true; }

# The same, for a rule: "  (NULL::uuid, 'country:BY', 1, ..." .
rule_rows_in() { grep -E "^  \(NULL::uuid, " "$1" || true; }

# --- 1. The migration's copy of the seed matches the generator's -------------
#
# Migration 005 embeds the rows rather than reading the file, because goose
# applies plain SQL and the seed has to run inside the same transaction as the
# DDL -- before row-level security is enabled, or the migrator is refused by its
# own policy. Embedding means two copies, and this is what keeps them one.
if ! diff <(rows_in "$seed_categories") <(rows_in "$migration") >/dev/null; then
  echo "check-taxonomy-seed: $migration no longer matches $seed_categories." >&2
  echo "  Re-run 'python eval/emit.py' and copy the INSERT block into the migration." >&2
  diff <(rows_in "$seed_categories") <(rows_in "$migration") | head -20 >&2
  status=1
fi

# --- 2. Migration 006's copy of the rules matches the generator's ------------
#
# Same reason as check 1, one table over. The rules seed additionally resolves
# a category code to an id through a JOIN, so a drifted copy does not
# necessarily fail to apply -- it can quietly seed fewer rules. The migration
# counts its own rows for that, and this keeps the two texts identical in the
# first place.
if ! diff <(rule_rows_in "$seed_rules") <(rule_rows_in "$migration_rules") >/dev/null; then
  echo "check-taxonomy-seed: $migration_rules no longer matches $seed_rules." >&2
  echo "  Re-run 'python eval/emit.py' and copy the INSERT block into the migration." >&2
  diff <(rule_rows_in "$seed_rules") <(rule_rows_in "$migration_rules") | head -20 >&2
  status=1
fi

# --- 3. Every rule points at a category that is seeded -----------------------
#
# A template rule belongs to no organisation, so it can only point at a shared
# category: every organisation holds its own id for its own copy of a
# per-organisation leaf, and one rule cannot name all of them. eval/build.py's
# assign_scopes makes anything a rule targets shared, and this is that invariant
# asserted from the outside, where change 3.2 will rely on it.
seeded_codes=$(rows_in "$seed_categories" | sed -E "s/^  \('v1', '([^']+)'.*/\1/" | sort -u)
used_codes=$(grep -oE "'[0-9]+', 'v1', 'v1', true\)" "$seed_rules" \
  | sed -E "s/^'([0-9]+)'.*/\1/" | sort -u)
# An empty result is a silent pass, and this check exists to not be silent:
# emit.py's row format has already changed once, when the rules seed grew a
# ruleset_version column.
if [[ -z $used_codes ]]; then
  echo "check-taxonomy-seed: no category codes found in $seed_rules --" >&2
  echo "  the generator's row format changed and this check stopped checking." >&2
  exit 1
fi

missing=$(comm -23 <(echo "$used_codes") <(echo "$seeded_codes"))
if [[ -n $missing ]]; then
  echo "check-taxonomy-seed: rules point at categories that are not seeded:" >&2
  echo "$missing" | sed 's/^/  /' >&2
  echo "  A shared rule cannot target a per-organisation category." >&2
  status=1
fi

# --- 4. No rule targets a section or a computed line -------------------------
#
# GM, NM, CM, IBT and NI are arithmetic over other lines; a transaction landing
# in one would be counted twice. A section is the sum of its children, so the
# same applies.
for code in $used_codes; do
  row=$(rows_in "$seed_categories" | grep -E "^  \('v1', '$code'," || true)
  [[ -z $row ]] && continue
  # ... level, is_leaf, is_pnl, is_computed, ...
  if [[ $row != *", true, "* ]]; then
    echo "check-taxonomy-seed: a rule targets $code, which is not a leaf" >&2
    status=1
  fi
  if [[ $row == *", true, NULL,"* && $row == *"true, true, NULL"* ]]; then
    echo "check-taxonomy-seed: a rule targets $code, which is a computed line" >&2
    status=1
  fi
done

if [[ $status -eq 0 ]]; then
  echo "check-taxonomy-seed: $(echo "$seeded_codes" | wc -l | tr -d ' ') categories, \
$(echo "$used_codes" | wc -l | tr -d ' ') of them targeted by rules, all consistent"
fi
exit $status
