// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisionertask

// StartInstanceParams contains the parameters for starting an instance.
// This is a simplified version for the FSM; real Juju uses environs.StartInstanceParams.
type StartInstanceParams struct {
	MachineID        string
	AvailabilityZone string
	// Additional fields will be added when wiring to real dependencies.
}

// StartInstanceResult contains the result of starting an instance.
type StartInstanceResult struct {
	InstanceID string
	ZoneName   string
}
