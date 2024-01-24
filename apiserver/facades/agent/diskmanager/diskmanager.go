// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package diskmanager

import (
	"context"

	"github.com/juju/names/v5"

	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/domain/blockdevice"
	"github.com/juju/juju/rpc/params"
)

type blockDeviceUpdater interface {
	UpdateBlockDevices(ctx context.Context, machineId string, devices ...blockdevice.BlockDevice) error
}

// DiskManagerAPI provides access to the DiskManager API facade.
type DiskManagerAPI struct {
	blockDeviceUpdater blockDeviceUpdater
	authorizer         facade.Authorizer
	getAuthFunc        common.GetAuthFunc
}

func (d *DiskManagerAPI) SetMachineBlockDevices(ctx context.Context, args params.SetMachineBlockDevices) (params.ErrorResults, error) {
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.MachineBlockDevices)),
	}
	canAccess, err := d.getAuthFunc()
	if err != nil {
		return result, err
	}
	for i, arg := range args.MachineBlockDevices {
		tag, err := names.ParseMachineTag(arg.Machine)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}
		if !canAccess(tag) {
			err = apiservererrors.ErrPerm
		} else {
			// TODO(axw) create volumes for block devices without matching
			// volumes, if and only if the block device has a serial. Under
			// the assumption of unique (to a machine) serial IDs, this
			// gives us a guaranteed *persistently* unique way of identifying
			// the volume.
			//
			// NOTE: we must predicate the above on there being no unprovisioned
			// volume attachments for the machine, otherwise we would have
			// a race between the volume attachment info being recorded and
			// the diskmanager publishing block devices and erroneously creating
			// volumes.
			err = d.blockDeviceUpdater.UpdateBlockDevices(ctx, tag.Id(), arg.BlockDevices...)
			// TODO(axw) set volume/filesystem attachment info.
		}
		result.Results[i].Error = apiservererrors.ServerError(err)
	}
	return result, nil
}
