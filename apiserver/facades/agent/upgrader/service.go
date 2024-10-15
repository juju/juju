// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrader

import (
	"context"

	"github.com/juju/version/v2"

	"github.com/juju/juju/core/machine"
)

// ModelAgentService provides access to the Juju agent version for the model.
type ModelAgentService interface {
	// GetMachineTargetAgentVersion reports the target agent version that should
	// be being run on the provided machine identified by name. The following
	// errors are possible:
	// - [github.com/juju/juju/domain/machine/errors.MachineNotFound]
	// - [github.com/juju/juju/domain/model/errors.NotFound]
	GetMachineTargetAgentVersion(context.Context, machine.Name) (version.Number, error)

	// GetModelTargetAgentVersion returns the target agent version for the
	// entire model. The following errors can be returned:
	// - [github.com/juju/juju/domain/model/errors.NotFound] - When the model does
	// not exist.
	GetModelTargetAgentVersion(context.Context) (version.Number, error)

	// GetUnitTargetAgentVersion reports the target agent version that should be
	// being run on the provided unit identified by name. The following errors
	// are possible:
	// - [github.com/juju/juju/domain/application/errors.UnitNotFound] - When
	// the unit in question does not exist.
	// - [github.com/juju/juju/domain/model/errors.NotFound] - When the model
	// the unit belongs to no longer exists.
	GetUnitTargetAgentVersion(context.Context, string) (version.Number, error)
}
