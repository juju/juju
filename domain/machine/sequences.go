// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machine

import "github.com/juju/juju/domain/sequence"

const (
	// MachineSequenceNamespace is the namespace for machine sequences.
	MachineSequenceNamespace = sequence.StaticNamespace("machine")

	// ContainerSequenceNamespace is the namespace for container sequences.
	ContainerSequenceNamespace = sequence.StaticNamespace("machine_container")
)
