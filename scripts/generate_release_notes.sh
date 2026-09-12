#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

TAG="${TAG:-}"
if [[ -z "$TAG" ]]; then
  TAG="$(git describe --tags --abbrev=0 2>/dev/null || true)"
fi

PREV_TAG="$(git tag -l 'v[0-9]*.[0-9]*.[0-9]*' --sort=-v:refname | grep -v "^${TAG}$" | head -n 1 || true)"

RANGE="HEAD"
if [[ -n "$PREV_TAG" ]]; then
  RANGE="${PREV_TAG}..${TAG:-HEAD}"
fi

echo "## What's Changed in ${TAG:-Release}"
echo ""

features=""
fixes=""
perf_refactor=""
others=""

re_feat='^feat(\([^)]+\))?:'
re_fix='^fix(\([^)]+\))?:'
re_perf_refactor='^(perf|refactor)(\([^)]+\))?:'

while IFS= read -r line; do
  [[ -z "$line" ]] && continue
  hash="$(echo "$line" | cut -d' ' -f1)"
  msg="$(echo "$line" | cut -d' ' -f2-)"

  if [[ "$msg" =~ $re_feat ]]; then
    clean_msg="$(echo "$msg" | sed -E 's/^feat(\([^)]+\))?:[[:space:]]*//')"
    features+="- ${clean_msg} (${hash})\n"
  elif [[ "$msg" =~ $re_fix ]]; then
    clean_msg="$(echo "$msg" | sed -E 's/^fix(\([^)]+\))?:[[:space:]]*//')"
    fixes+="- ${clean_msg} (${hash})\n"
  elif [[ "$msg" =~ $re_perf_refactor ]]; then
    clean_msg="$(echo "$msg" | sed -E 's/^(perf|refactor)(\([^)]+\))?:[[:space:]]*//')"
    perf_refactor+="- ${clean_msg} (${hash})\n"
  else
    others+="- ${msg} (${hash})\n"
  fi
done < <(git log --oneline --no-merges "$RANGE")

if [[ -n "$features" ]]; then
  echo "### Features"
  echo -e "$features"
fi

if [[ -n "$fixes" ]]; then
  echo "### Bug Fixes"
  echo -e "$fixes"
fi

if [[ -n "$perf_refactor" ]]; then
  echo "### Performance & Refactoring"
  echo -e "$perf_refactor"
fi

if [[ -n "$others" ]]; then
  echo "### Maintenance & Documentation"
  echo -e "$others"
fi

echo "---"
echo "**Full Changelog**: https://github.com/SakagamiJun/lightnovel-tui/commits/${TAG:-HEAD}"
