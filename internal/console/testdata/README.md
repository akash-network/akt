# Console API OpenAPI contract

`openapi.json` is the OpenAPI 3.0 document for the Akash Console API
(`https://console-api.akash.network`), vendored verbatim from the Console
repository's `apps/api/swagger/openapi.json`. It is consumed by
`contract_test.go`, which validates every request the `internal/console`
client produces against this contract.

The Variables & Secrets snapshot matches commit
`1758b470e327084d6fce863126e87b221d4fcf68` byte for byte:
[pinned source](https://raw.githubusercontent.com/akash-network/console/1758b470e327084d6fce863126e87b221d4fcf68/apps/api/swagger/openapi.json).
Its SHA-256 is
`e26cc837f62a73919e90103a4b12237d95c6d567c0ab63b296e0ec8be28369ef`.

## Refreshing

1. Fetch `apps/api/swagger/openapi.json` from a pinned Console repository
   revision and record the revision and checksum here.
2. Replace this file verbatim — do not hand-edit it.
3. Run `GOWORK=off go test ./internal/console/` and fix any client methods
   that the contract test now flags.
