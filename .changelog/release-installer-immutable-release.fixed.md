- Fix the release workflow failing to publish `install.sh`: GitHub immutable
  releases are sealed when GoReleaser publishes them, so the later
  `gh release upload` was rejected (HTTP 422) and v1.0.3 shipped without an
  installer. Releases now use GoReleaser Pro's split release: `make
  release-prepare` builds without touching GitHub, the workflow generates,
  dry-runs, and attests the installer, and `make release-publish` publishes it
  together with the archives before updating the Homebrew formula. Publishing
  refuses an installer that was not generated from the prepared release's
  checksum manifest.
- Dry-run the release installer before publication through
  `script/release-installer-dry-run.sh`, whose `curl` stand-in serves only the
  release's download URLs from the prepared files, and again against the
  published release. Manual snapshot and dry-run modes run the same check, and
  the dry-run mode now runs the same prepare step as publication.
