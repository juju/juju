// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package errors

import "github.com/juju/juju/internal/errors"

const (
	// AgentVersionNotFound describes an error that occurs
	// when an agent version record is not present.
	AgentVersionNotFound = errors.ConstError("agent version not found")

	// MachineAgentVersionNotSet describes an error that occurs when a machine
	// does not have its agent version set.
	MachineAgentVersionNotSet = errors.ConstError("machine agent version not set")

	// MissingAgentBinaries describes an error that occurs when agent binaries
	// are missing for a given entity that runs agent binaries within the
	// model, eg units and machines. When agent binaries are missing, it
	// means that the model does not have a copy of the binaries.
	MissingAgentBinaries = errors.ConstError("missing agent binaries")

	// UnitAgentVersionNotSet describes an error that occurs when a unit
	// does not have its agent version set.
	UnitAgentVersionNotSet = errors.ConstError("unit agent version not set")
)
