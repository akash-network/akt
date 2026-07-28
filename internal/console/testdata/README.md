# Console API OpenAPI contract

`openapi.json` is the OpenAPI 3.0 document for the Akash Console API
(`https://console-api.akash.network`), vendored verbatim from the Console API
via the `console-axi` reference repository. It is consumed by
`contract_test.go`, which validates every request the `internal/console`
client produces against this contract.

## Refreshing

1. Regenerate/fetch the current `openapi.json` from the Console API's
   `console-axi` reference repo (the API's generated OpenAPI document).
2. Replace this file verbatim — do not hand-edit it.
3. Run `GOWORK=off go test ./internal/console/` and fix any client methods
   that the contract test now flags.

## Known upstream quirk

44 `dseq` schemas carry the pattern `^d+$` (literal letter `d`), an escaping
artifact of the intended `^\d+$` (digits) — the correctly escaped `^\\d+$`
appears elsewhere in the same document. `contract_test.go` normalizes this at
load time; the vendored file is kept byte-for-byte as published.
