package console

import (
	"encoding/json"
	"fmt"

	sdkclient "github.com/cosmos/cosmos-sdk/client"
	"github.com/spf13/cobra"

	"pkg.akt.dev/akt/internal/actionlog"
	"pkg.akt.dev/akt/internal/cliutil"
	aktctx "pkg.akt.dev/akt/internal/context"
)

// Organization and project management drops the project header: listing,
// creating, or renaming projects is organization-wide, and a stale project
// selection must not lock the user out of the commands that repair it.

func organizationCmds(mgrFn func() *aktctx.Manager) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "organization",
		Aliases: []string{"org", "team"},
		RunE:    sdkclient.ValidateCmd,
		Short:   "Show the Console organization (team) the API key acts in",
		Long: "A Console organization is a team or a personal account. An API key acts in the " +
			"organization it was created in, with the role its owner holds there. Creating a " +
			"team and accepting an invitation need a signed-in session in the Console web app.",
	}

	cmd.AddCommand(&cobra.Command{
		Use:     "list",
		Short:   "List the organization the API key acts in, with your role there",
		Args:    cobra.NoArgs,
		Example: `  akt console organization list`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cl, _, err := clientFromCmd(cmd, mgrFn, true)
			if err != nil {
				return err
			}

			organizations, err := cl.ForProject("").ListOrganizations(cmd.Context())
			if err != nil {
				return fmt.Errorf("list organizations: %w", err)
			}

			return printJSON(cmd, organizations)
		},
	})

	return cmd
}

func projectCmds(mgrFn func() *aktctx.Manager) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "project",
		RunE:  sdkclient.ValidateCmd,
		Short: "Manage the projects of the Console organization",
		Long: "Projects group an organization's deployments. Every organization has a default " +
			"project. A project argument is the project's id, slug, or name. Select the project " +
			"a context acts in with `akt console project use <project>`, or for one command with " +
			"--console-project or AKT_CONSOLE_PROJECT.",
	}

	cmd.AddCommand(
		projectListCmd(mgrFn),
		projectGetCmd(mgrFn),
		projectCreateCmd(mgrFn),
		projectRenameCmd(mgrFn),
		projectDeleteCmd(mgrFn),
		projectUseCmd(mgrFn),
	)

	return cmd
}

func projectListCmd(mgrFn func() *aktctx.Manager) *cobra.Command {
	return &cobra.Command{
		Use:     "list",
		Short:   "List the projects the API key can reach",
		Args:    cobra.NoArgs,
		Example: `  akt console project list`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cl, _, err := clientFromCmd(cmd, mgrFn, true)
			if err != nil {
				return err
			}

			projects, err := cl.ForProject("").ListProjects(cmd.Context())
			if err != nil {
				return fmt.Errorf("list projects: %w", err)
			}

			return printJSON(cmd, projects)
		},
	}
}

func projectGetCmd(mgrFn func() *aktctx.Manager) *cobra.Command {
	return &cobra.Command{
		Use:     "get <project>",
		Short:   "Show a project, its active deployments, and what you may do in it",
		Args:    cobra.ExactArgs(1),
		Example: `  akt console project get checkout-api`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, _, err := clientFromCmd(cmd, mgrFn, true)
			if err != nil {
				return err
			}

			project, err := cl.FindProject(cmd.Context(), args[0])
			if err != nil {
				return fmt.Errorf("get project %s: %w", args[0], err)
			}

			return printJSON(cmd, project)
		},
	}
}

func projectCreateCmd(mgrFn func() *aktctx.Manager) *cobra.Command {
	return &cobra.Command{
		Use:   "create <name> [description]",
		Short: "Create a project (owners and admins)",
		Args:  cobra.RangeArgs(1, 2),
		Example: `  akt console project create checkout-api
  akt console project create checkout-api "Payments services"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, _, err := clientFromCmd(cmd, mgrFn, true)
			if err != nil {
				return err
			}

			description := ""
			if len(args) > 1 {
				description = args[1]
			}

			project, err := cl.ForProject("").CreateProject(cmd.Context(), args[0], description)
			if err != nil {
				return fmt.Errorf("create project %s: %w", args[0], err)
			}

			return printJSON(cmd, project)
		},
	}
}

func projectRenameCmd(mgrFn func() *aktctx.Manager) *cobra.Command {
	return &cobra.Command{
		Use:     "rename <project> <new-name>",
		Short:   "Rename a project; its id and slug stay the same",
		Args:    cobra.ExactArgs(2),
		Example: `  akt console project rename checkout-api payments`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, _, err := clientFromCmd(cmd, mgrFn, true)
			if err != nil {
				return err
			}
			cl = cl.ForProject("")

			target, err := cl.FindProject(cmd.Context(), args[0])
			if err != nil {
				return fmt.Errorf("rename project %s: %w", args[0], err)
			}

			project, err := cl.RenameProject(cmd.Context(), target.ID, args[1])
			if err != nil {
				return fmt.Errorf("rename project %s: %w", args[0], err)
			}

			return printJSON(cmd, project)
		},
	}
}

func projectDeleteCmd(mgrFn func() *aktctx.Manager) *cobra.Command {
	return &cobra.Command{
		Use:     "delete <project>",
		Short:   "Delete a project that holds no open deployment (not the default project)",
		Args:    cobra.ExactArgs(1),
		Example: `  akt console project delete checkout-api`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, _, err := clientFromCmd(cmd, mgrFn, true)
			if err != nil {
				return err
			}
			cl = cl.ForProject("")

			target, err := cl.FindProject(cmd.Context(), args[0])
			if err != nil {
				return fmt.Errorf("delete project %s: %w", args[0], err)
			}

			if err := cl.DeleteProject(cmd.Context(), target.ID); err != nil {
				return fmt.Errorf("delete project %s: %w", args[0], err)
			}

			return printConsoleResult(cmd, fmt.Sprintf("Project %s (%s) deleted.", target.Name, target.ID), struct {
				ID      string `json:"id"      yaml:"id"`
				Name    string `json:"name"    yaml:"name"`
				Deleted bool   `json:"deleted" yaml:"deleted"`
			}{ID: target.ID, Name: target.Name, Deleted: true})
		},
	}
}

func projectUseCmd(mgrFn func() *aktctx.Manager) *cobra.Command {
	return &cobra.Command{
		Use:   "use <project>",
		Short: "Make the context act in a project and its organization",
		Long: "Store the project's id and its organization's id on the context, so Console " +
			"commands, `akt deploy`, and `akt mcp` act in that project. --console-project and " +
			"AKT_CONSOLE_PROJECT still override it for one command. Clear the selection with " +
			"`akt context edit <context> --console-project \"\" --console-organization \"\"`.",
		Args:    cobra.ExactArgs(1),
		Example: `  akt console project use checkout-api`,
		RunE: func(cmd *cobra.Command, args []string) error {
			m := mgrFn()
			if m == nil {
				return fmt.Errorf("project use requires a context to store the selection")
			}
			rc, err := resolveContextFromCmd(cmd, m)
			if err != nil {
				return fmt.Errorf("project use requires a context to store the selection: %w", err)
			}

			cl, _, err := clientFromCmd(cmd, mgrFn, true)
			if err != nil {
				return err
			}

			project, err := cl.FindProject(cmd.Context(), args[0])
			if err != nil {
				return fmt.Errorf("use project %s: %w", args[0], err)
			}
			organization, err := cl.ForProject(project.ID).ActiveOrganization(cmd.Context())
			if err != nil {
				return fmt.Errorf("use project %s: %w", args[0], err)
			}

			err = m.UpdateContext(rc.Name, func(c *aktctx.Context) error {
				c.ConsoleOrganization = organization.ID
				c.ConsoleProject = project.ID
				return nil
			})
			recordProjectSelection(cmd, map[string]string{
				"console-organization": organization.ID,
				"console-project":      project.ID,
			}, err)
			if err != nil {
				return err
			}

			return printConsoleResult(cmd,
				fmt.Sprintf("Context %q now acts in project %s (%s) of organization %s (%s).",
					rc.Name, project.Name, project.ID, organization.Name, organization.ID),
				struct {
					Context          string `json:"context"          yaml:"context"`
					OrganizationID   string `json:"organizationId"   yaml:"organizationId"`
					OrganizationName string `json:"organizationName" yaml:"organizationName"`
					ProjectID        string `json:"projectId"        yaml:"projectId"`
					ProjectName      string `json:"projectName"      yaml:"projectName"`
				}{rc.Name, organization.ID, organization.Name, project.ID, project.Name},
			)
		},
	}
}

// recordProjectSelection records the context change in the selected
// context's action log (SPEC §5.6), like `akt context edit` does.
func recordProjectSelection(cmd *cobra.Command, details map[string]string, opErr error) {
	logger := cliutil.ActionLogFromContext(cmd.Context())
	if logger == nil {
		return
	}

	entry := actionlog.Entry{Type: actionlog.TypeContext, Action: "edit", Status: "success"}
	if params, err := json.Marshal(details); err == nil {
		entry.Params = params
	}
	if opErr != nil {
		entry.Status = "failed"
		entry.Error = opErr.Error()
	}

	_ = logger.Log(entry)
}

func deploymentMoveCmd(mgrFn func() *aktctx.Manager) *cobra.Command {
	return &cobra.Command{
		Use:     "move <dseq> <project>",
		Short:   "File a deployment in another project (owners and admins)",
		Args:    cobra.ExactArgs(2),
		Example: `  akt console deployment move 12345 checkout-api`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, _, err := clientFromCmd(cmd, mgrFn, true)
			if err != nil {
				return err
			}

			target, err := cl.FindProject(cmd.Context(), args[1])
			if err != nil {
				return fmt.Errorf("move deployment %s: %w", args[0], err)
			}

			moved, err := cl.MoveDeployment(cmd.Context(), args[0], target.ID)
			if err != nil {
				return fmt.Errorf("move deployment %s to project %s: %w", args[0], target.Name, err)
			}

			return printConsoleResult(cmd,
				fmt.Sprintf("Deployment %s moved to project %s (%s).", moved.DSeq, target.Name, target.ID),
				moved,
			)
		},
	}
}
