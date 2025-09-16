// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package diskmanager

import (
	"context"

	"github.com/juju/names/v6"

	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/core/blockdevice"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/machine"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/rpc/params"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package diskmanager -destination service_mock_test.go github.com/juju/juju/apiserver/facades/agent/diskmanager MachineService,BlockDeviceService

type MachineService interface {
	// GetMachineUUID returns the UUID of a machine identified by its name.
	GetMachineUUID(ctx context.Context, name machine.Name) (machine.UUID, error)
}

type BlockDeviceService interface {
	// UpdateBlockDevicesForMachine updates the block devices for the specified
	// machine.
	UpdateBlockDevicesForMachine(
		ctx context.Context, machineUUID machine.UUID,
		devices []blockdevice.BlockDevice,
	) error
}

// DiskManagerAPI provides access to the DiskManager API facade.
type DiskManagerAPI struct {
	machineService     MachineService
	blockDeviceService BlockDeviceService
	authorizer         facade.Authorizer
	getAuthFunc        common.GetAuthFunc
}

func (d *DiskManagerAPI) SetMachineBlockDevices(
	ctx context.Context, args params.SetMachineBlockDevices,
) (params.ErrorResults, error) {
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.MachineBlockDevices)),
	}
	canAccess, err := d.getAuthFunc(ctx)
	if err != nil {
		return result, err
	}
	one := func(arg params.MachineBlockDevices) error {
		tag, err := names.ParseMachineTag(arg.Machine)
		if err != nil {
			return apiservererrors.ErrPerm
		}
		if !canAccess(tag) {
			return apiservererrors.ErrPerm
		}

		machineUUID, err := d.machineService.GetMachineUUID(
			ctx, machine.Name(tag.Id()))
		if errors.Is(err, machineerrors.MachineNotFound) {
			return errors.Errorf(
				"machine %q not found", tag.Id(),
			).Add(coreerrors.NotFound)
		} else if err != nil {
			return err
		}

		blockdevices := blockDevicesFromParams(arg.BlockDevices)
		err = d.blockDeviceService.UpdateBlockDevicesForMachine(
			ctx, machineUUID, blockdevices)
		if errors.Is(err, machineerrors.MachineNotFound) {
			return errors.Errorf(
				"machine %q not found", tag.Id(),
			).Add(coreerrors.NotFound)
		} else if err != nil {
			return err
		}

		return nil
	}
	for i, arg := range args.MachineBlockDevices {
		err := one(arg)
		result.Results[i].Error = apiservererrors.ServerError(err)
	}
	return result, nil
}

func blockDevicesFromParams(in []params.BlockDevice) []blockdevice.BlockDevice {
	if len(in) == 0 {
		return nil
	}
	out := make([]blockdevice.BlockDevice, len(in))
	for i, d := range in {
		out[i] = blockdevice.BlockDevice{
			DeviceName:      d.DeviceName,
			DeviceLinks:     d.DeviceLinks,
			FilesystemLabel: d.Label,
			FilesystemUUID:  d.UUID,
			HardwareId:      d.HardwareId,
			WWN:             d.WWN,
			BusAddress:      d.BusAddress,
			SizeMiB:         d.SizeMiB,
			FilesystemType:  d.FilesystemType,
			InUse:           d.InUse,
			MountPoint:      d.MountPoint,
			SerialId:        d.SerialId,
		}
	}
	return out
}
