- Work in Akash Console teams and projects. A context can store a Console
  team and project (`akt console project use`), and `--console-organization`,
  `--console-project`, `AKT_CONSOLE_ORGANIZATION`, and `AKT_CONSOLE_PROJECT`
  override them for one command. Console calls send the `x-organization-id`
  and `x-project-id` headers only when a value is selected, so accounts without
  teams see no change.
- Add `akt console organization list` (alias `team list`) and
  `akt console project list/get/create/rename/delete/use`. `akt deploy` and
  `akt console deployment create/list` act in the selected project, and
  `akt console deployment move <dseq> <project>` files a deployment in another
  project.
- `akt console apikey create --console-project` creates a key limited to one
  project, and `apikey list` shows the team and project of each key.
  `akt console whoami` shows the active team, role, and project.
- Explain the Console team refusals `project_required`, `project_forbidden`,
  `organization_mismatch`, `project_budget_exceeded`, and
  `project_limit_reached` with the command that fixes each. Provider gateway commands mint a token naming the deployment for
  keys limited to some projects.
- `akt mcp` accepts `--console-organization` and `--console-project`, adds the
  `console_list_organizations` and `console_list_projects` tools, and the
  Console deployment tools take an optional `project`. A server started with
  a project refuses tool calls that name another one.
