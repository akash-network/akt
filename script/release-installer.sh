#!/usr/bin/env bash
# Generate the release install.sh and its attestation subject list from the
# checksum manifest GoReleaser wrote (SPEC §12.7). The installer is pinned to
# the manifest's tag and embeds each archive's SHA-256, so verifying the
# installer's attestation also vouches for the archive it installs. The
# committed binstaller config is copied, never modified.
set -euo pipefail

if [[ $# -lt 4 || $# -gt 5 ]]; then
  echo "usage: release-installer.sh BINST CONFIG DIST OUT_DIR [EXPECTED_TAG]" >&2
  exit 1
fi
binst=$1
config=$2
dist=$3
out=$4
expected_tag=${5:-}

installer=$out/install.sh
subjects=$out/attestation-subjects.txt

# Neither an earlier run's outputs nor a partial write may survive a failed
# run, or a later step could attest them.
remove_outputs() {
  rm -f -- "$installer" "$subjects"
}
on_exit() {
  local status=$?
  if [[ $status -ne 0 ]]; then
    remove_outputs
  fi
  exit "$status"
}
trap on_exit EXIT
mkdir -p -- "$out"
remove_outputs

shopt -s nullglob
manifests=("$dist"/akt_*_checksums.txt)
shopt -u nullglob
if [[ ${#manifests[@]} -ne 1 ]]; then
  echo "expected exactly one akt_<version>_checksums.txt in $dist, found ${#manifests[@]}" >&2
  exit 1
fi
manifest=${manifests[0]}
manifest_name=${manifest##*/}
version=${manifest_name#akt_}
version=${version%_checksums.txt}
tag=v$version
if [[ -n "$expected_tag" && "$expected_tag" != "$tag" ]]; then
  echo "$manifest_name belongs to $tag, not $expected_tag" >&2
  exit 1
fi

cp -- "$config" "$out/binstaller.yml"
"$binst" embed-checksums --config "$out/binstaller.yml" --mode checksum-file --file "$manifest" --version "$tag" --quiet
"$binst" gen --config "$out/binstaller.yml" --target-version "$tag" -o "$installer"

sha256() {
  if command -v sha256sum >/dev/null; then
    sha256sum "$1"
  else
    shasum -a 256 "$1"
  fi
}

# One subject per line in shasum format: every archive and package GoReleaser
# checksummed, the manifest itself, and the installer. awk restores a final
# newline the manifest may lack so its last entry stays a separate line.
{
  awk 1 "$manifest"
  (cd -- "$dist" && sha256 "$manifest_name")
  (cd -- "$out" && sha256 install.sh)
} > "$subjects"
