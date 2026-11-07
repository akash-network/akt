- Stabilize the monitor subscription-rejection test by receiving the full
  subscription batch before returning its JSON-RPC error, then verifying
  client-initiated closure. This prevents a peer disconnect from masking the
  expected rejection with a broken pipe during coverage runs.
