// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrader

import (
	"context"

	"github.com/juju/juju/cloud"
	coreagentbinary "github.com/juju/juju/core/agentbinary"
	"github.com/juju/juju/core/credential"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/semversion"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/environs/config"
)

// ModelAgentService provides access to the Juju agent version for the model.
type ModelAgentService interface {
	// GetModelTargetAgentVersion returns the target agent version for the
	// entire model. The following errors can be returned:
	// - [github.com/juju/juju/domain/model/errors.NotFound] - When the model does
	// not exist.
	GetModelTargetAgentVersion(context.Context) (semversion.Number, error)

	// SetMachineReportedAgentVersion sets the reported agent version for the
	// supplied machine name. Reported agent version is the version that the
	// agent binary on this machine has reported it is running.
	//
	// The following errors are possible:
	// - [coreerrors.NotValid] if the reportedVersion is not valid or the machine
	// name is not valid.
	// - [coreerrors.NotSupported] if the architecture is not supported.
	// - [machineerrors.MachineNotFound] when the machine does not exist.
	// - [machineerrors.MachineDead] when the machine is dead.
	SetMachineReportedAgentVersion(context.Context, machine.Name, coreagentbinary.Version) error

	// SetUnitReportedAgentVersion sets the reported agent version for the
	// supplied unit name. Reported agent version is the version that the agent
	// binary on this unit has reported it is running.
	//
	// The following errors are possible:
	// - [coreerrors.NotValid] - when the reportedVersion is not valid.
	// - [coreerrors.NotSupported] - when the architecture is not supported.
	// - [applicationerrors.UnitNotFound] - when the unit does not exist.
	// - [applicationerrors.UnitIsDead] - when the unit is dead.
	SetUnitReportedAgentVersion(context.Context, coreunit.Name, coreagentbinary.Version) error

	// WatchMachineTargetAgentVersion is responsible for watching the target agent
	// version for machine and reporting when there has been a change via a
	// [watcher.NotifyWatcher]. The following errors can be expected:
	// - [machineerrors.NotFound] - When no machine exists for the provided name.
	// - [modelerrors.NotFound] - When the model of the machine no longer exists.
	WatchMachineTargetAgentVersion(ctx context.Context, machineName machine.Name) (watcher.NotifyWatcher, error)

	// WatchModelTargetAgentVersion is responsible for watching the target agent
	// version of this model and reporting when a change has happened in the
	// version.
	WatchModelTargetAgentVersion(ctx context.Context) (watcher.NotifyWatcher, error)

	// WatchUnitTargetAgentVersion is responsible for watching the target agent
	// version for unit and reporting when there has been a change via a
	// [watcher.NotifyWatcher]. The following errors can be expected:
	// - [applicationerrors.UnitNotFound] - When no unit exists for the provided name.
	// - [modelerrors.NotFound] - When the model of the unit no longer exists.
	WatchUnitTargetAgentVersion(ctx context.Context, unitName coreunit.Name) (watcher.NotifyWatcher, error)
}

// CredentialService provides access to credentials.
type CredentialService interface {
	CloudCredential(ctx context.Context, key credential.Key) (cloud.Credential, error)
}

type ControllerNodeService interface {
	// SetControllerNodeReportedAgentVersion sets the agent version for the
	// supplied controllerID. Version represents the version of the controller
	// node's agent binary.
	//
	// The following errors are possible:
	// - [coreerrors.NotValid] if the version is not valid.
	// - [coreerrors.NotSupported] if the architecture is not supported.
	// - [controllernodeerrors.NotFound] if the controller node does not exist.
	SetControllerNodeReportedAgentVersion(context.Context, string, coreagentbinary.Version) error
}

// ModelConfigService is an interface that provides access to the
// model configuration.
type ModelConfigService interface {
	// ModelConfig returns the current config for the model.
	ModelConfig(ctx context.Context) (*config.Config, error)
}
