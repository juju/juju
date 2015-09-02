// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package workload

import (
	"gopkg.in/juju/charm.v5"
)

// Plugin represents the functionality of a workload plugin.
type Plugin interface {
	// Launch runs the plugin's "launch" command, passing the provided
	// workload definition. The output is converted to a Details.
	Launch(definition charm.Workload) (Details, error)

	// Destroy runs the plugin's "destroy" command for the given ID.
	Destroy(id string) error

	// Status runs the plugin's "status" command. The output is
	// converted to a PluginStatus.
	Status(id string) (PluginStatus, error)
}
