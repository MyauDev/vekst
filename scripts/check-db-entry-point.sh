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
