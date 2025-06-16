// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lifeflag

import (
	"context"

	"github.com/juju/names/v6"

	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/state"
)

// MachineService defines the methods that the facade assumes from the Machine
// service.
type MachineService interface {
	// GetMachineUUID returns the UUID of a machine identified by its name.
	GetMachineUUID(ctx context.Context, name machine.Name) (machine.UUID, error)
	// InstanceID returns the cloud specific instance id for this machine.
	InstanceID(ctx context.Context, mUUID machine.UUID) (instance.Id, error)
	// GetMachineLife returns the lifecycle state of the machine with the
	// specified name.
	GetMachineLife(ctx context.Context, name machine.Name) (life.Value, error)
}

// ApplicationService defines the methods that the facade assumes from the
// Application service.
type ApplicationService interface {
	// GetUnitLife returns the lifecycle state of the unit with the
	// specified name.
	GetUnitLife(ctx context.Context, name unit.Name) (life.Value, error)
}

// Backend represents the interface required for this facade to retried entity
// information.
type Backend interface {
	state.EntityFinder
}

// NewFacade constructs a new life flag facade.
func NewFacade(
	applicationService ApplicationService,
	machineService MachineService,
	backend Backend,
	watcherRegistry facade.WatcherRegistry,
	authorizer facade.Authorizer,
	logger logger.Logger,
) (*Facade, error) {
	if !authorizer.AuthUnitAgent() && authorizer.AuthApplicationAgent() {
		return nil, apiservererrors.ErrPerm
	}
	getCanAccess := func(context.Context) (common.AuthFunc, error) {
		return func(tag names.Tag) bool {
			return authorizer.AuthOwner(tag)
		}, nil
	}
	life := common.NewLifeGetter(applicationService, machineService, backend, getCanAccess, logger)
	watch := common.NewAgentEntityWatcher(backend, watcherRegistry, getCanAccess)
	return &Facade{
		LifeGetter:         life,
		AgentEntityWatcher: watch,
	}, nil
}

type Facade struct {
	*common.LifeGetter
	*common.AgentEntityWatcher
}
