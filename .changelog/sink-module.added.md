- Add the sink module (AKT to Solana transfers) to the chain commands.
  `akt tx sink` covers deposits, the pause guardian, reporter outcome
  reports, vetoes and refunds, plus governance proposals that update
  parameters, order a void, veto a void or resolve a held transfer.
  `akt q sink` covers a transfer, paginated transfers and disputes, reporter
  policies, account usage, totals and parameters.
- Validate sink input before broadcast the way the module does: exact uakt
  amounts (fractional uakt is rejected, never truncated), composite transfer
  IDs, base58 Solana addresses and 64-byte signatures, reason length, and
  resolution evidence. Reports read the current reporter policy and transfer
  round from the chain unless `--policy-version` and `--round` are given.
  Governance voids and resolutions bind to an expected settlement round.
- Render sink queries and messages in pretty output with full addresses,
  Solana keys and signatures, and register the sink codec so proposals and
  transactions that carry sink messages decode.
- Pin the published SDK module to the same commit as node `feat/sink-review`
  and use its existing descriptor registration for Amino signing.
