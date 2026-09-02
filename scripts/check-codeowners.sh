#!/usr/bin/env bash
# Fails if .github/CODEOWNERS still contains placeholder text.
#
# This file's own header used to carry "TODO(0.1): @MyauDev/track-a and
# @MyauDev/track-b are placeholders... or every rule below silently matches
# nobody" -- and for a while, an uncommitted edit renamed the placeholders
# without resolving the TODO, which this check would have caught. A rule
# that can silently match nobody is not a rule; this is the mechanism that
# keeps a future edit from reintroducing that quietly.
set -euo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")/.."

if grep -qiE 'TODO|track-a|track-b|placeholder' .github/CODEOWNERS; then
  echo "::error::.github/CODEOWNERS still contains placeholder text:"
  grep -inE 'TODO|track-a|track-b|placeholder' .github/CODEOWNERS
  exit 1
fi
