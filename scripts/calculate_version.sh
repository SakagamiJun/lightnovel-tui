#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

INPUT_BUMP="${BUMP:-auto}"

# If triggered by a manual tag push (e.g. v1.0.0), output it directly
if [[ "${GITHUB_REF_TYPE:-}" == "tag" && "${GITHUB_REF_NAME:-}" == v* ]]; then
  echo "Triggered by manual tag: ${GITHUB_REF_NAME}"
  echo "tag=${GITHUB_REF_NAME}"
  echo "version=${GITHUB_REF_NAME#v}"
  echo "skip=false"
  exit 0
fi

# Fetch tags if in git repository
git fetch --tags --force 2>/dev/null || true

last_tag="$(git tag -l 'v[0-9]*.[0-9]*.[0-9]*' --sort=-v:refname | head -n 1 || true)"
base_version="0.0.0"
range="HEAD"

if [[ -n "$last_tag" ]]; then
  base_version="${last_tag#v}"
  range="${last_tag}..HEAD"
fi

bump="$INPUT_BUMP"
if [[ "$bump" == "auto" || -z "$bump" ]]; then
  commit_lines="$(git log --format='%s%n%b' "$range" 2>/dev/null || true)"
  bump=""
  if grep -Eq '^feat(\([^)]+\))?:' <<< "$commit_lines"; then
    bump="minor"
  elif grep -Eq '^(fix|perf|refactor)(\([^)]+\))?:' <<< "$commit_lines"; then
    bump="patch"
  fi

  if [[ -z "$bump" ]]; then
    echo "No releasable commits found since ${last_tag:-repository start}. Skipping release." >&2
    echo "skip=true"
    exit 0
  fi
fi

IFS='.' read -r major minor patch <<< "$base_version"
case "$bump" in
  major) major=$((major + 1)); minor=0; patch=0 ;;
  minor) minor=$((minor + 1)); patch=0 ;;
  patch) patch=$((patch + 1)) ;;
  *) echo "Unknown bump type: $bump" >&2; exit 1 ;;
esac

tag="v${major}.${minor}.${patch}"
version="${major}.${minor}.${patch}"

echo "Calculated next version: ${version} (${tag})" >&2
echo "tag=${tag}"
echo "version=${version}"
echo "skip=false"
