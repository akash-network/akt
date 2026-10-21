# Setup and discovery

Use an installed `akt` on PATH. In an akt source checkout, build with
`GOWORK=off make akt` and use `.cache/bin/akt`. The parent Go workspace is not
part of the akt module. Released binaries do not need Go or the source tree.

Skill packages are named `akt_<version>_skill.zip`. Check the release checksum,
extract the `akt-cli/` folder, and place that whole folder in the agent's
supported skill directory. For Codex repository scope, that is
`.agents/skills/akt-cli/`; user scope is `~/.agents/skills/akt-cli/`.
Keep the references and `agents/openai.yaml` with `SKILL.md`. Installing the
skill does not install the binary, choose a context, or configure credentials.

## Inspect a context

`example` is a placeholder context name throughout these recipes. Choose an
existing context matching the requested account and network before creating one.

```bash
akt context show --context example -o json
```

Inspect `name`, `network`, `auth_method`, `default_account`, and `capabilities`.
`console_api_key_configured` reports presence without disclosing the key.
Existing `AKT_CONSOLE_API_KEY` overrides the context credential, so check the
selected identity with `console whoami` when using a process-level credential.
Keep the credential itself out of output and command examples.

## Create a Console context

Only create a context when the task needs a new one. Console-only operation
needs no network or local signing key:

```bash
akt context create example --deploy-via console
```

## Authenticate Console

An API key can be supplied securely as `AKT_CONSOLE_API_KEY` by the execution
environment. For a user at a terminal, `akt console login --context example`
prompts for a key with hidden input and stores it for that context.
For a process-provided key, verify the identity directly:

```bash
akt console whoami --context example -o json
akt console wallet balance --context example -o json
```

Console funds deployments from account credits. Do not pass a USD or coin
deposit to a Console deployment. A deployment's runtime limit is distinct from
the account's Auto Recharge settings; consult `console deployment settings
--help` when the user wants a runtime limit.

## Work in a Console team

An API key acts in the team (organization) it was created in, with its owner's
role there. Check which team, role, and project the context acts in, and which
projects the key can reach:

```bash
akt console whoami --context example -o json
akt console project list --context example -o json
```

`whoami` reports `organization` with the caller's `role`, and `project`, which
is null when the context acts across every project the key reaches. Neither
field appears for an account without teams. Select a project for the context
only when the user names one:

```bash
akt console project use checkout-api --context example
```

The context stores the project's id and its team's id. For one command, pass
`--console-project <project>` (id, slug, or name), or `--console-project=` to
act across every project; `AKT_CONSOLE_PROJECT` and `AKT_CONSOLE_ORGANIZATION`
do the same for a process. They never change the stored selection.

Creating a team, deleting or leaving one, and accepting an invitation need the
Console web app; no API key can do them. A key limited to one project is made
with `akt console apikey create <name> --console-project <project>`; it prints
the new secret once, so keep that output out of logs. Its project cannot be
changed later.

## Create a chain context

First inspect `akt context network list` and `akt context keys list --help`.
If the intended network already exists, create a context referencing it and
the user's existing account:

```bash
akt context create example --network mainnet --default-account alice
```

Here `mainnet` and `alice` must already name the intended network and account.
Use `akt context network create --help` when a network must be configured.
Key creation or recovery is a separate user task; it can produce sensitive
backup material. Do not create a replacement wallet just to bypass an unlock
or missing-account error. Use the configured secure keyring backend.

For an existing context, `akt context edit example --deploy-via chain` or
`--deploy-via console` changes the shared workflow preference. It does not
disable the other credential. Prefer the context already selected by the user.

## Optional MCP connection

Configure a client that supports local stdio MCP servers to launch this command
and argument list:

```json
{
  "command": "akt",
  "args": ["mcp", "--context", "example"]
}
```

The client's surrounding configuration format is client-specific. The process
must have access to the intended akt home and credentials. Stdout belongs to
the protocol. Discover the registered tools rather than assuming every CLI
operation is exposed. Writes require an explicitly authorized
`--enable-writes` configuration and a context for action logging; retain the
read-only default for inspection.
