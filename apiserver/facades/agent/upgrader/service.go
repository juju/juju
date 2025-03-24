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
	// GetMachineTargetAgentVersion reports the target agent version that should
	// be being run on the provided machine identified by name. The following
	// errors are possible:
	// - [github.com/juju/juju/domain/machine/errors.MachineNotFound]
	// - [github.com/juju/juju/domain/model/errors.NotFound]
	GetMachineTargetAgentVersion(context.Context, machine.Name) (semversion.Number, error)

	// GetModelTargetAgentVersion returns the target agent version for the
	// entire model. The following errors can be returned:
	// - [github.com/juju/juju/domain/model/errors.NotFound] - When the model does
	// not exist.
	GetModelTargetAgentVersion(context.Context) (semversion.Number, error)

	// GetUnitTargetAgentVersion reports the target agent version that should be
	// being run on the provided unit identified by name. The following errors
	// are possible:
	// - [github.com/juju/juju/domain/application/errors.UnitNotFound] - When
	// the unit in question does not exist.
	// - [github.com/juju/juju/domain/model/errors.NotFound] - When the model
	// the unit belongs to no longer exists.
	GetUnitTargetAgentVersion(context.Context, string) (semversion.Number, error)

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
	WatchUnitTargetAgentVersion(ctx context.Context, unitName string) (watcher.NotifyWatcher, error)
}

// CredentialService provides access to credentials.
type CredentialService interface {
	CloudCredential(ctx context.Context, key credential.Key) (cloud.Credential, error)
}

type ControllerNodeService interface {
	SetReportedControllerAgentVersion(context.Context, string, coreagentbinary.Version) error
}

type MachineService interface {
	// SetReportedMachineAgentVersion sets the reported agent version for the
	// supplied machine name. Reported agent version is the version that the agent
	// binary on this machine has reported it is running.
	//
	// The following errors are possible:
	// - [coreerrors.NotValid] if the reportedVersion is not valid.
	// - [coreerrors.NotSupported] if the architecture is not supported.
	// - [machineerrors.MachineNotFound] - when the machine does not exist.
	SetReportedMachineAgentVersion(context.Context, machine.Name, coreagentbinary.Version) error
}

type UnitService interface {
	// SetReportedUnitAgentVersion sets the reported agent version for the
	// supplied unit name. Reported agent version is the version that the agent
	// binary on this unit has reported it is running.
	//
	// The following errors are possible:
	// - [coreerrors.NotValid] if the reportedVersion is not valid.
	// - [coreerrors.NotSupported] if the architecture is not supported.
	// - [applicationerrors.UnitNotFound] - when the unit does not exist.
	SetReportedUnitAgentVersion(context.Context, coreunit.Name, coreagentbinary.Version) error
}

// ModelConfigService is an interface that provides access to the
// model configuration.
type ModelConfigService interface {
	// ModelConfig returns the current config for the model.
	ModelConfig(ctx context.Context) (*config.Config, error)
}
