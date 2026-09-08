#!/usr/bin/env bash
# Fails if any Go file outside core/internal/db imports pgxpool directly.
#
# That import is the one door openspec design D2 requires: InTx in
# core/internal/db/tx.go is the only place a transaction begins, so nothing
# else -- handler or River worker alike -- may hold a *pgxpool.Pool or query
# it directly. This is the mechanism, not the policy; the policy is in
# CLAUDE.md.
set -euo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")/.."

# core/internal/jobs is the one sanctioned second door: River's own driver
# needs the raw pool for background polling and leader election, which
# touches only River's infrastructure tables -- tables design D4 establishes
# carry no tenant data. See core/internal/jobs/jobs.go's package doc.
offenders=$(grep -rl '"github.com/jackc/pgx/v5/pgxpool"' --include='*.go' core \
  | grep -v '^core/internal/db/' \
  | grep -v '^core/internal/jobs/' || true)

if [ -n "$offenders" ]; then
  echo "::error::pgxpool imported outside core/internal/db (design D2 -- InTx is the only entry point):"
  echo "$offenders"
  exit 1
fi

# --- Committed call-site counts -------------------------------------------
#
# Three of the doors into tenancy are counted rather than forbidden. Counting
# is the right shape for a door that has legitimate users but should not gain
# new ones quietly: the number is committed here, so adding a call site is a
# diff in this file that a reviewer sees, and forgetting to update it fails
# the build.
#
# InSystemTx opens a transaction with NO tenant context. Its legitimate
# callers read only tables that belong to no tenant -- goose_db_version, the
# four identity tables -- plus OrgIDForSession, which is the lookup that
# turns a session into an organisation and so by definition runs before one
# is known. See the doc comment on InSystemTx.
#
# OrgIDFromJobArgs and OrgIDForNewOrg are the two OrgID constructors that do
# NOT start from an authenticated session, so each one is a place where the
# authority to act for a tenant comes from somewhere other than a resolved
# membership. OrgIDForSession is deliberately uncounted: it is the door that
# is supposed to be used, and counting it would only punish normal work.
#
# Test files are excluded throughout -- a test that needs a tenant gets one
# from OrgIDForTest, and counting harness call sites would make the numbers
# noise rather than signal.
expect_count() {
  name=$1
  want=$2
  pattern=$3
  # `|| true` at each stage: grep exits 1 when it selects nothing, and under
  # `set -o pipefail` that would abort the script rather than report a count
  # of zero -- which is a legitimate expected value.
  got=$( { grep -rn "$pattern" --include='*.go' core || true; } \
    | { grep -v '_test.go' || true; } \
    | { grep -vE '^core/internal/db/(orgid|tx)\.go:[0-9]+:func ' || true; } \
    | wc -l | tr -d ' ')
  if [ "$got" != "$want" ]; then
    echo "::error::$name has $got call site(s), expected $want."
    echo "If the new one is deliberate, update the count in $0 in the same commit:"
    { grep -rn "$pattern" --include='*.go' core || true; } \
      | { grep -v '_test.go' || true; } \
      | { grep -vE '^core/internal/db/(orgid|tx)\.go:[0-9]+:func ' || true; }
    exit 1
  fi
}

expect_count InSystemTx       8 'InSystemTx('
expect_count OrgIDFromJobArgs 1 'OrgIDFromJobArgs('
expect_count OrgIDForNewOrg   1 'OrgIDForNewOrg()'

# --- The tenant context is set in exactly one place ------------------------
#
# db.InTx issues the only set_config('app.org_id', ...) in the codebase. A
# second one anywhere else is a second way to become a tenant, and one that
# no test and no reviewer is watching.
setters=$( { grep -rn "set_config(\|SET LOCAL\|SET app\." --include='*.go' core || true; } \
  | { grep -v '_test.go' || true; } \
  | { grep -vE '^core/internal/db/tx\.go:' || true; } \
  | { grep -vE ':[0-9]+:[[:space:]]*//' || true; } )
if [ -n "$setters" ]; then
  echo "::error::the tenant context is set outside core/internal/db/tx.go (design D2):"
  echo "$setters"
  exit 1
fi

# --- The tenant identifier is bound, never interpolated --------------------
#
# SET does not take bind parameters, so the tempting way to write this is
# string concatenation of a tenant identifier into SQL text. set_config() is
# a function call and does take one, which is why InTx uses it. This catches
# the alternative: any app.org_id appearing in a formatted or concatenated
# string.
interpolated=$( { grep -rn "app\.org_id" --include='*.go' core || true; } \
  | { grep -v '_test.go' || true; } \
  | { grep -vE ':[0-9]+:[[:space:]]*//' || true; } \
  | { grep -E "Sprintf|\+ *org|org *\+|%s|%v" || true; } )
if [ -n "$interpolated" ]; then
  echo "::error::a tenant identifier is interpolated into SQL text; bind it instead:"
  echo "$interpolated"
  exit 1
fi
