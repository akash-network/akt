- Fix provider REST certificate verification for JWT and mTLS by querying the
  provider's valid on-chain certificate after standard PKI verification fails.
  Cover public status and authenticated gateway clients, retain named wallet
  accounts during mTLS setup, and verify manifest submission, retrieval, and
  lease status against local TLS fixtures.
