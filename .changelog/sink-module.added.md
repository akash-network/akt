- Add the sink module (AKT to Solana transfers) to the chain commands.
  `akt tx sink` covers deposits, the pause guardian, reporter outcome
  reports, vetoes and refunds, plus governance proposals that update
  parameters, order a void, veto a void or resolve a held transfer.
  `akt q sink` covers records, transfers, open transfers (with
  `--held-only`), account usage, totals and parameters.
- Validate sink input before broadcast the way the module does: exact uakt
  amounts (fractional uakt is rejected, never truncated), canonical transfer
  IDs, base58 Solana addresses and 64-byte signatures, reason length, and
  resolution evidence. Reports read the current reporter epoch and transfer
  round from the chain unless `--reporter-epoch` and `--round` are given.
- Render sink queries and messages in pretty output with full addresses,
  Solana keys and signatures, and register the sink codec so proposals and
  transactions that carry sink messages decode.
- Build against the private chain-sdk branch that adds the sink API until a
  tagged `pkg.akt.dev/go` release includes it.
