// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migration

import (
	"context"

	coreagentbinary "github.com/juju/juju/core/agentbinary"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/semversion"
	coreunit "github.com/juju/juju/core/unit"
)

// ModelAgentService provides access to the Juju agent version for the model.
type ModelAgentService interface {
	// GetMachineReportedAgentVersion returns the agent binary version that was last
	// reported to be running by the agent on the machine.
	// The following errors are possible:
	// - [github.com/juju/juju/domain/errors.MachineNotFound] when the machine
	// being asked for does not exist.
	// - [github.com/juju/juju/domain/modelagent/errors.AgentVersionNotFound]
	// when no agent version has been reported for the given machine.
	GetMachineReportedAgentVersion(context.Context, machine.Name) (coreagentbinary.Version, error)

	// GetModelTargetAgentVersion returns the target agent version for the
	// entire model. The following errors can be returned:
	// - [github.com/juju/juju/domain/modelagent/errors.NotFound] when the model
	// does not exist.
	GetModelTargetAgentVersion(context.Context) (semversion.Number, error)

	// GetUnitReportedAgentVersion returns the agent binary version that was last
	// reported to be running by the agent on the unit.
	// The following errors are possible:
	// - [github.com/juju/juju/domain/application/errors.UnitNotFound] when the
	// unit in question does not exist.
	// - [github.com/juju/juju/domain/modelagent/errors.AgentVersionNotFound]
	// when no agent version has been reported for the given machine.
	GetUnitReportedAgentVersion(context.Context, coreunit.Name) (coreagentbinary.Version, error)
}
