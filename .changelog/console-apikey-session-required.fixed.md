- Explain why `akt console apikey create` fails against a Console that creates
  API keys only from a signed-in session. Console answers a creation request
  authenticated with an API key `403` with code `session_required`; the client
  now returns a `SessionRequiredError` for it, records the create as failed rather
  than pending, and the command names where to create the key (Console >
  Settings > API Keys) and how to store it (`akt console login`). The live
  sandbox lifecycle checks that refusal when Console enforces it and keeps the
  create and delete round trip otherwise.
