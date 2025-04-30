// Copyright 2014 Cloudbase Solutions SRL
// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package reboot

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v6"

	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/domain/machine/service"
)

// RebootAPI provides access to the Reboot API facade.
type RebootAPI struct {
	*common.RebootActionGetter
	// The ability for a machine to reboot itself is not yet used.
	*common.RebootRequester
	*common.RebootFlagClearer
	*common.MachineWatcher
}

// NewRebootApiFromModelContext creates a new server-side RebootAPI facade.
func NewRebootApiFromModelContext(ctx facade.ModelContext) (*RebootAPI, error) {
	return NewRebootAPI(
		ctx.Auth(),
		ctx.WatcherRegistry(),
		ctx.DomainServices().Machine())
}

// NewRebootAPI creates a new instance of the RebootAPI by initializing the various components needed for reboot functionality.
//   - [facade.Authorizer] to authorize the machine agent,
//   - [facade.WatcherRegistry] to register and manage watchers,
//   - [github.com/juju/juju/domain/machine/service.WatchableService] for interacting with machine-related data and operations.
func NewRebootAPI(
	auth facade.Authorizer,
	watcherRegistry facade.WatcherRegistry,
	machineService *service.WatchableService,
) (*RebootAPI, error) {
	if !auth.AuthMachineAgent() {
		return nil, apiservererrors.ErrPerm
	}

	tag, ok := auth.GetAuthTag().(names.MachineTag)
	if !ok {
		return nil, errors.Errorf("Expected names.MachineTag, got %T", auth.GetAuthTag())
	}

	canAccess := func(context.Context) (common.AuthFunc, error) {
		return auth.AuthOwner, nil
	}

	// TODO: ask to simon or joe if we should introduce a kind of function GetMachineUUIDFromTag in domain to avoid the check below
	if tag.Kind() != names.MachineTagKind {
		return nil, errors.Errorf("%q should be a %s", tag, names.MachineTagKind)
	}

	uuid := func(ctx context.Context) (machine.UUID, error) {
		uuid, err := machineService.GetMachineUUID(ctx, machine.Name(tag.Id()))
		if err != nil {
			return "", errors.Annotatef(err, "find machine uuid for machine %q", tag.Id())
		}
		return uuid, nil
	}

	return &RebootAPI{
		RebootActionGetter: common.NewRebootActionGetter(machineService, canAccess),
		RebootRequester:    common.NewRebootRequester(machineService, canAccess),
		RebootFlagClearer:  common.NewRebootFlagClearer(machineService, canAccess),
		MachineWatcher:     common.NewMachineRebootWatcher(machineService, watcherRegistry, uuid),
	}, nil
}
