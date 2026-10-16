- Publish an attested `install.sh` with every release. The release workflow
  pins it to the tag with the archive checksums embedded, dry-runs it against
  the published assets for every supported platform, and attests it together
  with every archive, package, and the checksum manifest before uploading it,
  so `gh attestation verify --signer-workflow` against `release.yml` succeeds.
- Fix the binstaller configuration to install the universal macOS archive,
  pin `binst` in the Makefile, and fail CI and the release quality gate when
  the committed `install.sh` drifts from its configuration.
- Attest the repository-root `install.sh`, which accepts any tag, from a
  dedicated `installer-attest` workflow that runs only for `main`. A contract
  test limits attestation-writing permission to it and `release.yml`.
