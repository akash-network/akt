- Format micro-denom amounts above the int64 range instead of panicking.
  Sink parameters on uncapped networks use the uint64 maximum, which made
  `FormatCoin` panic in `Int64()`; amounts are now compared as `math.Int`.
