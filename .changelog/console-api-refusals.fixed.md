- Stop reporting a lease as created when its provider refused the manifest
  as invalid. Console answers `provider_manifest_invalid` (422) once the lease
  exists; `akt` now fails with a message that sending the manifest again
  cannot help and that the deployment should be closed and redeployed with a
  corrected SDL, on both the default and `--manifest` paths.
- Treat Console's `deposit_price_unavailable` (503) on a deployment create as a
  refusal made before signing. `akt` used to drop the body of every 5xx and
  report an unknown outcome; it now says nothing was signed and to run the
  same command again.
- Refuse project names, project descriptions, and API key names that Console
  rejects (control or bidi characters, or no visible character) before
  sending them, naming the offending code point.
- Pin the vendored Console API contract to `console-api/v3.332.0` and compile
  Console's JavaScript-style name patterns so the contract test enforces them.
