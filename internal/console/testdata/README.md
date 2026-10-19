# Console API OpenAPI contract

`openapi.json` is the OpenAPI 3.0 document for the Akash Console API
(`https://console-api.akash.network`), vendored verbatim from the Console
repository's `apps/api/swagger/openapi.json`. It is consumed by
`contract_test.go`, which validates every request the `internal/console`
client produces against this contract.

The teams and projects snapshot matches commit
`5037f6c90e1f0d7811a5da398420a1a4d09474b6` byte for byte:
[pinned source](https://raw.githubusercontent.com/akash-network/console/5037f6c90e1f0d7811a5da398420a1a4d09474b6/apps/api/swagger/openapi.json).
Its SHA-256 is
`999dbb6291a1feccc74d91b967169411997609367129e7d6b7fe2c77c0b10b6c`.

The committed document lists routes that the served `/v1/doc` reference
still hides behind a feature flag, among them the organization (team) and
project routes and the `x-organization-id` and `x-project-id` headers.

## Refreshing

1. Fetch `apps/api/swagger/openapi.json` from a pinned Console repository
   revision and record the revision and checksum here.
2. Replace this file verbatim — do not hand-edit it.
3. Run `GOWORK=off go test ./internal/console/` and fix any client methods
   that the contract test now flags.
