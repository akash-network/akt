#!/usr/bin/env bash
# Print whether the PR needs Console E2E. Unknown changes require it.
set -euo pipefail

if [[ $# != 2 ]]; then
  echo "usage: ci-console-required.sh BASE_SHA HEAD_SHA" >&2
  exit 1
fi
for revision in "$@"; do
  if [[ ! "$revision" =~ ^[0-9a-fA-F]{40}$ ]]; then
    echo "Console change selection requires full commit SHAs" >&2
    exit 1
  fi
done

# Materialize the diff so a Git failure cannot look like an empty path list.
# Disabling renames checks both the deleted source and added destination.
changed_paths=$(mktemp)
trap 'rm -f -- "$changed_paths"' EXIT
git diff --no-ext-diff --no-textconv --no-renames --name-only -z "$1...$2" -- > "$changed_paths"

saw_change=false
while IFS= read -r -d '' changed_path; do
  saw_change=true
  case "$changed_path" in
    README.md|AGENTS.md|AICHANGELOG.md|DESIGN.md|SPEC.md|CONTRIBUTING.md|LICENSE|docs/*.md|.changelog/*.md)
      ;;
    internal/output/pretty/bme.go|internal/output/pretty/bme*_test.go|internal/output/pretty/testdata/TestRenderBME*/*.golden)
      # These render chain BME responses, not Console responses. Do not extend
      # this to shared pretty helpers or the whole output package.
      ;;
    *)
      printf 'true\n'
      exit 0
      ;;
  esac
done < "$changed_paths"

if [[ "$saw_change" == true ]]; then
  printf 'false\n'
else
  # An unexpected empty PR diff is not evidence that Console is unaffected.
  printf 'true\n'
fi
