package commands

import "strings"

// Command represents a single entry in the command palette.
type Command struct {
	Name        string   // display name shown in the palette
	Description string   // short description shown next to the name
	Category    string   // grouping: "navigation", "action", "context", "app"
	Aliases     []string // alternative strings that match during filtering
}

// Registry holds all available commands and supports filtering.
type Registry struct {
	commands []Command
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{}
}

// Register adds a command to the registry.
func (r *Registry) Register(cmd Command) {
	r.commands = append(r.commands, cmd)
}

// All returns every registered command.
func (r *Registry) All() []Command {
	out := make([]Command, len(r.commands))
	copy(out, r.commands)
	return out
}

// Filter returns commands whose Name or any Alias contains the query
// (case-insensitive substring match). An empty query returns all commands.
func (r *Registry) Filter(query string) []Command {
	if query == "" {
		return r.All()
	}

	q := strings.ToLower(query)

	var out []Command
	for _, cmd := range r.commands {
		if strings.Contains(strings.ToLower(cmd.Name), q) {
			out = append(out, cmd)
			continue
		}

		for _, alias := range cmd.Aliases {
			if strings.Contains(strings.ToLower(alias), q) {
				out = append(out, cmd)
				break
			}
		}
	}

	return out
}

// DefaultRegistry returns a registry pre-populated with all default commands.
func DefaultRegistry() *Registry {
	r := NewRegistry()

	// Navigation commands.
	r.Register(Command{Name: "Dashboard", Description: "Go to dashboard", Category: "navigation", Aliases: []string{"home"}})
	r.Register(Command{Name: "Deployments", Description: "View all deployments", Category: "navigation", Aliases: []string{"dep"}})
	r.Register(Command{Name: "Leases", Description: "View leases", Category: "navigation"})
	r.Register(Command{Name: "Providers", Description: "View providers", Category: "navigation", Aliases: []string{"prov"}})
	r.Register(Command{Name: "Validators", Description: "View validators", Category: "navigation", Aliases: []string{"val"}})
	r.Register(Command{Name: "Governance", Description: "View governance proposals", Category: "navigation", Aliases: []string{"gov"}})
	r.Register(Command{Name: "Certificates", Description: "View certificates", Category: "navigation", Aliases: []string{"cert"}})
	r.Register(Command{Name: "Escrow", Description: "View escrow accounts", Category: "navigation"})
	r.Register(Command{Name: "Orders", Description: "View orders", Category: "navigation"})
	r.Register(Command{Name: "Bids", Description: "View bids", Category: "navigation"})
	r.Register(Command{Name: "Monitor", Description: "Real-time network monitor", Category: "navigation", Aliases: []string{"monitor", "top", "consensus"}})
	r.Register(Command{Name: "Query", Description: "Query commands panel", Category: "navigation"})
	r.Register(Command{Name: "Tx", Description: "Transaction commands panel", Category: "navigation"})

	// Action commands.
	r.Register(Command{Name: "Deploy", Description: "Create new deployment", Category: "action"})

	// Application commands.
	r.Register(Command{Name: "Quit", Description: "Quit application", Category: "app", Aliases: []string{"q", "exit"}})
	r.Register(Command{Name: "Help", Description: "Show help", Category: "app", Aliases: []string{"?"}})

	return r
}
