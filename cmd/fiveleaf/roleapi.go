package main

import (
	"encoding/json"

	"fiveleaf/internal/cli"
)

func (m *fiveleaf) serveRoles(request *cli.RoleRequest) cli.Reply {
	switch request.Action {
	case "list", "export":
		if request.Bundle != nil {
			return cli.Reply{Error: "role export takes no bundle"}
		}
		bundle, err := exportRoleBundle(m.s.Prefs)
		if err != nil {
			return cli.Reply{Error: err.Error()}
		}
		return cli.Reply{Data: bundle}
	case "import":
		beforeRoles, beforePresets := len(m.s.AgentRoles), len(m.s.AgentRolePresets)
		if err := m.importAgentFile(roleFile, request.Bundle); err != nil {
			return cli.Reply{Error: err.Error()}
		}
		result, _ := json.Marshal(map[string]int{"created": len(m.s.AgentRoles) - beforeRoles, "presetsApplied": len(m.s.AgentRolePresets) - beforePresets})
		return cli.Reply{Data: result}
	default:
		return cli.Reply{Error: "unknown role action: use list, export or import"}
	}
}
