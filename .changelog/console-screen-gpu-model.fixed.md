- Fix Console bid screening GPU model filters by sending the model in the
  resource attribute key with value `true`, including when overriding an SDL.
  Return an empty array in JSON/YAML when no providers match, while preserving
  the pretty output message. Add request and empty-output regression tests.
