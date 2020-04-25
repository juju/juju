// Copyright 2016 Canonical Ltd.
// Copyright 2016 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package actions

import (
	"github.com/juju/charm/v7"
)

// JujuRunActionName defines the action name used by juju-run.
const JujuRunActionName = "juju-run"

// PredefinedActionsSpec defines a spec for each predefined action.
var PredefinedActionsSpec = map[string]charm.ActionSpec{
	JujuRunActionName: {
		Description: "predefined juju-run action",
		Params: map[string]interface{}{
			"type":        "object",
			"title":       JujuRunActionName,
			"description": "predefined juju-run action params",
			"required":    []interface{}{"command", "timeout"},
			"properties": map[string]interface{}{
				"command": map[string]interface{}{
					"type":        "string",
					"description": "command to be ran under juju-run",
				},
				"timeout": map[string]interface{}{
					"type":        "number",
					"description": "timeout for command execution",
				},
				"workload-context": map[string]interface{}{
					"type":        "boolean",
					"description": "run the command in k8s workload context",
				},
			},
		},
	},
}
