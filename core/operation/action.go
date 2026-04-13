// Copyright 2016 Canonical Ltd.
// Copyright 2016 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package operation

import (
	"strings"

	"github.com/juju/juju/domain/deployment/charm"
)

// JujuExecActionName defines the action name used by juju-exec.
const JujuExecActionName = "juju-exec"

// legacyJujuRunActionName will be removed in Juju 4.
const legacyJujuRunActionName = "juju-run"

// IsJujuExecAction returns true if name is the "juju-exec" action.
func IsJujuExecAction(name string) bool {
	// Check for the legacy "juju-run" as well in case an upgrade was
	// done and actions had been previously queued.
	return name == JujuExecActionName || name == legacyJujuRunActionName
}

// HasJujuExecAction returns true if the "juju-exec" binary name appears
// anywhere in the specified commands.
func HasJujuExecAction(commands string) bool {
	return strings.Contains(commands, JujuExecActionName) || strings.Contains(commands, legacyJujuRunActionName)
}

// PredefinedActionsSpec defines a spec for each predefined action.
var PredefinedActionsSpec = map[string]charm.ActionSpec{
	JujuExecActionName: {
		Description: "predefined juju-exec action",
		Parallel:    true,
		Params: map[string]any{
			"type":        "object",
			"title":       JujuExecActionName,
			"description": "predefined juju-exec action params",
			"required":    []any{"command", "timeout"},
			"properties": map[string]any{
				"command": map[string]any{
					"type":        "string",
					"description": "command to be ran under juju-exec",
				},
				"timeout": map[string]any{
					"type":        "number",
					"description": "timeout for command execution",
				},
				"workload-context": map[string]any{
					"type":        "boolean",
					"description": "run the command in k8s workload context",
				},
			},
		},
	},
}
