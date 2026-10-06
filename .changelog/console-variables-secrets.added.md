- console-variables-secrets: Add encrypted Console secrets, saved SDL reads,
  partial deployment updates, and redeploy with secret inheritance.
- Verify malformed inputs, encryption failures, and partial outcomes with
  hermetic coverage of the deployment configuration workflows.
- Reject patches that could mutate shared YAML endpoint or service definitions.
