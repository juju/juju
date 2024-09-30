// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v5"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/core/machine"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

// InstanceIdGetter implements a common InstanceId method for use by
// various facades.
type InstanceIdGetter struct {
	machineService MachineService
	getCanRead     GetAuthFunc
}

// NewInstanceIdGetter returns a new InstanceIdGetter. The GetAuthFunc
// will be used on each invocation of InstanceId to determine current
// permissions.
func NewInstanceIdGetter(st state.EntityFinder, machineService MachineService, getCanRead GetAuthFunc) *InstanceIdGetter {
	return &InstanceIdGetter{
		machineService: machineService,
		getCanRead:     getCanRead,
	}
}

// InstanceId returns the provider specific instance id for each given
// machine or an CodeNotProvisioned error, if not set.
func (ig *InstanceIdGetter) InstanceId(ctx context.Context, args params.Entities) (params.StringResults, error) {
	result := params.StringResults{
		Results: make([]params.StringResult, len(args.Entities)),
	}
	canRead, err := ig.getCanRead()
	if err != nil {
		return result, err
	}
	for i, entity := range args.Entities {
		tag, err := names.ParseTag(entity.Tag)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}
		if canRead(tag) {
			machineUUID, err := ig.machineService.GetMachineUUID(ctx, machine.Name(tag.Id()))
			if errors.Is(err, machineerrors.MachineNotFound) {
				result.Results[i].Error = apiservererrors.ServerError(errors.NotFoundf("machine %s", tag.Id()))
				continue
			} else if err != nil {
				result.Results[i].Error = apiservererrors.ServerError(err)
				continue
			}
			instanceId, err := ig.machineService.InstanceID(ctx, machineUUID)
			if errors.Is(err, machineerrors.NotProvisioned) {
				result.Results[i].Error = apiservererrors.ServerError(errors.NotProvisionedf("machine %s", tag.Id()))
				continue
			} else if err != nil {
				result.Results[i].Error = apiservererrors.ServerError(err)
				continue
			}
			result.Results[i].Result = instanceId
		} else {
			result.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
		}
	}
	return result, nil
}
