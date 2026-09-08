#!/usr/bin/env bash
# Fails if a query outside core/internal/db/query/identity.sql names one of the
# four identity tables, or if a statement inside it reaches outside them.
#
# users, user_identities, sessions and auth_flows are exempt from row-level
# security (deploy/db/rls-exempt-tables.txt), each for a reason recorded there:
# none of them belongs to an organisation. That exemption means nothing at the
# database level filters what a query on them returns, so the boundary has to be
# drawn somewhere else. This is that somewhere -- the same move as
# check-db-entry-point.sh, which turns "InTx is the only door" from a rule in a
# document into a failing build.
#
# The exposure being prevented is cross-person, not cross-tenant: an unfiltered
# read of users returns every user of every customer, which in a product whose
# customers are named companies discloses who those companies are. See openspec
# add-identity design 1.1.
set -euo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")/.."

QUERY_DIR="core/internal/db/query"
IDENTITY_FILE="$QUERY_DIR/identity.sql"
IDENTITY_TABLES="users user_identities sessions auth_flows"

if [ ! -f "$IDENTITY_FILE" ]; then
  echo "::error::$IDENTITY_FILE is missing; this check has nothing to enforce"
  exit 1
fi

status=0

# 1. No other query file may name these tables. Reading them from elsewhere is
#    how an unfiltered join or a forgotten predicate gets written.
#
#    Comments are stripped first, the same way check 2 below does it. The rule
#    is about statements, not prose: a file whose comment explains *why* it
#    deliberately does not join users -- which is precisely the reasoning worth
#    writing down -- was failing this check, and the only way to satisfy it was
#    to delete the explanation. A check that punishes the documentation of its
#    own rule teaches people to remove it.
for table in $IDENTITY_TABLES; do
  offenders=""
  for f in "$QUERY_DIR"/*.sql; do
    [ "$f" = "$IDENTITY_FILE" ] && continue
    if sed 's/--.*$//' "$f" | grep -qEi "\\b${table}\\b"; then
      offenders="${offenders}${f}"$'\n'
    fi
  done
  if [ -n "$offenders" ]; then
    echo "::error::identity table '${table}' is named outside ${IDENTITY_FILE}:"
    echo "$offenders"
    status=1
  fi
done

# 2. Nothing inside identity.sql may reach outside the four. Strip comments,
#    then take the first token after each FROM / JOIN / INTO / UPDATE and check
#    it against the allowlist. sqlc's own files are small and comment-heavy, so
#    dropping "--" lines first matters.
referenced=$(sed 's/--.*$//' "$IDENTITY_FILE" \
  | tr 'A-Z' 'a-z' \
  | grep -oE '\b(from|join|into|update)[[:space:]]+[a-z_][a-z0-9_]*' \
  | awk '{print $2}' \
  | sort -u)

for table in $referenced; do
  case " $IDENTITY_TABLES " in
    *" $table "*) ;;
    *)
      echo "::error::${IDENTITY_FILE} references '${table}', which is not one of: ${IDENTITY_TABLES}"
      status=1
      ;;
  esac
done

exit $status
