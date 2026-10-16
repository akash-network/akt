#!/usr/bin/env bash
# Dry-run (`-n`) the release install.sh for one platform (SPEC §12.7) and
# require it to verify the archive with a checksum embedded in the script.
#
#   release-installer-dry-run.sh INSTALLER OS/ARCH
#   release-installer-dry-run.sh INSTALLER OS/ARCH DIST REPOSITORY TAG
#
# The first form runs against the public release. The second runs before the
# release exists: a curl stand-in, first on PATH, serves exactly
# https://github.com/REPOSITORY/releases/download/TAG/<asset> from DIST and
# fails every other request, so the installer fetches the bytes GoReleaser
# will upload from the URL they will have once published. The installer never
# sees GITHUB_TOKEN, matching a user's anonymous download.
set -euo pipefail

usage() {
  echo "usage: release-installer-dry-run.sh INSTALLER OS/ARCH [DIST REPOSITORY TAG]" >&2
  exit 1
}

if [[ $# -ne 2 && $# -ne 5 ]]; then
  usage
fi
installer=$1
platform=$2
if [[ ! "$platform" =~ ^[a-z0-9]+/[a-z0-9]+$ ]]; then
  echo "platform must be OS/ARCH, got: $platform" >&2
  exit 1
fi
if [[ ! -f "$installer" ]]; then
  echo "installer not found: $installer" >&2
  exit 1
fi

run_env=(PATH="$PATH")
if [[ $# -eq 5 ]]; then
  dist=$3
  repository=$4
  tag=$5
  if [[ ! -d "$dist" ]]; then
    echo "dist directory not found: $dist" >&2
    exit 1
  fi
  if [[ ! "$repository" =~ ^[A-Za-z0-9._-]+/[A-Za-z0-9._-]+$ ]]; then
    echo "repository must be OWNER/NAME, got: $repository" >&2
    exit 1
  fi
  if [[ -z "$tag" || "$tag" == */* ]]; then
    echo "invalid tag: $tag" >&2
    exit 1
  fi

  standin=$(mktemp -d)
  trap 'rm -rf -- "$standin"' EXIT
  # install.sh calls `curl -fsSL [-H header] -o FILE URL`. Values arrive
  # through the environment so no path or URL is spliced into the script.
  cat >"$standin/curl" <<'EOF'
#!/bin/sh
out=''
url=''
while [ $# -gt 0 ]; do
  case "$1" in
  -o) out=$2; shift 2 ;;
  -H) shift 2 ;;
  -*) shift ;;
  *) url=$1; shift ;;
  esac
done
case "$url" in
"$AKT_DRY_RUN_RELEASE_URL"*) name=${url#"$AKT_DRY_RUN_RELEASE_URL"} ;;
*) echo "curl stand-in: refusing $url; only $AKT_DRY_RUN_RELEASE_URL<asset> is served" >&2; exit 22 ;;
esac
case "$name" in
'' | */*) echo "curl stand-in: refusing $url; only $AKT_DRY_RUN_RELEASE_URL<asset> is served" >&2; exit 22 ;;
esac
if [ ! -f "$AKT_DRY_RUN_DIST/$name" ]; then
  echo "curl stand-in: $name is not in $AKT_DRY_RUN_DIST" >&2
  exit 22
fi
if [ -n "$out" ]; then
  cp -- "$AKT_DRY_RUN_DIST/$name" "$out"
else
  cat -- "$AKT_DRY_RUN_DIST/$name"
fi
EOF
  chmod +x "$standin/curl"
  run_env=(
    PATH="$standin:$PATH"
    AKT_DRY_RUN_RELEASE_URL="https://github.com/$repository/releases/download/$tag/"
    AKT_DRY_RUN_DIST="$(cd -- "$dist" && pwd -P)"
  )
fi

if ! log=$(env -u GITHUB_TOKEN "${run_env[@]}" \
  BINSTALLER_OS="${platform%/*}" BINSTALLER_ARCH="${platform#*/}" BINSTALLER_NO_PROGRESS=1 \
  sh "$installer" -n 2>&1); then
  printf '%s\n' "$log" >&2
  echo "$platform: release installer dry run failed" >&2
  exit 1
fi
printf '%s\n' "$log"
if ! grep -qF 'Using embedded checksum for verification' <<<"$log"; then
  echo "$platform: archive was not verified with an embedded checksum" >&2
  exit 1
fi
