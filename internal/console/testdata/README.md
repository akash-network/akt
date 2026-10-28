# Console API OpenAPI contract

`openapi.json` is the OpenAPI 3.0 document for the Akash Console API
(`https://console-api.akash.network`), vendored verbatim from the Console
repository's `apps/api/swagger/openapi.json`. It is consumed by
`contract_test.go`, which validates every request the `internal/console`
client produces against this contract.

The snapshot matches commit
`77f1b6c7f3d12c4cc75470eedc771a15b9de0fbe` (the `console-api/v3.332.0`
release) byte for byte:
[pinned source](https://raw.githubusercontent.com/akash-network/console/77f1b6c7f3d12c4cc75470eedc771a15b9de0fbe/apps/api/swagger/openapi.json).
Its SHA-256 is
`ef50a5a5945fca9ca72aecec135d47aa5268d8e79e2d166d7791bcbec19ba6b8`.

Console writes its schema patterns for JavaScript (`\u001f` escapes), so
`contract_test.go` compiles them through `compileECMAScriptPattern` rather
than kin-openapi's default, which would fail on the lowercase form.

## Refreshing

1. Fetch `apps/api/swagger/openapi.json` from a pinned Console repository
   revision and record the revision and checksum here.
2. Replace this file verbatim; do not hand-edit it.
3. Run `GOWORK=off go test ./internal/console/` and fix any client methods
   that the contract test now flags.
