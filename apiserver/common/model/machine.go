// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"context"

	"github.com/juju/errors"

	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/status"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

// ModelMachineInfo returns information about machine hardware for
// alive top level machines (not containers).
func ModelMachineInfo(ctx context.Context, st ModelManagerBackend, machineService MachineService, statusService StatusService) (machineInfo []params.ModelMachineInfo, _ error) {
	machines, err := st.AllMachines()
	if err != nil {
		return nil, errors.Trace(err)
	}

	machineStatuses, err := statusService.GetAllMachineStatuses(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}

	for _, m := range machines {
		if m.Life() != state.Alive {
			continue
		}
		machineName := machine.Name(m.Id())

		var aStatus string
		var statusMessage string
		machineStatus, ok := machineStatuses[machineName]
		if ok {
			aStatus = string(machineStatus.Status)
			statusMessage = machineStatus.Message
		} else {
			aStatus = string(status.Unknown)
		}

		mInfo := params.ModelMachineInfo{
			Id:      m.Id(),
			Status:  aStatus,
			Message: statusMessage,
		}
		machineUUID, err := machineService.GetMachineUUID(ctx, machine.Name(m.Id()))
		if errors.Is(err, machineerrors.MachineNotFound) {
			return nil, errors.NotFoundf("machine %q", m.Id())
		} else if err != nil {
			return nil, errors.Trace(err)
		}
		instanceID, displayName, err := machineService.GetInstanceIDAndName(ctx, machineUUID)
		switch {
		case err == nil:
			mInfo.InstanceId = instanceID.String()
			mInfo.DisplayName = displayName
		case errors.Is(err, machineerrors.MachineNotFound):
			return nil, errors.NotFoundf("machine %q", m.Id())
		case errors.Is(err, machineerrors.NotProvisioned):
			// ok, but no instance ID to get.
		default:
			return nil, errors.Trace(err)
		}
		if m.ContainerType() != "" && m.ContainerType() != instance.NONE {
			machineInfo = append(machineInfo, mInfo)
			continue
		}
		// Only include cores for physical machines.
		hw, err := machineService.GetHardwareCharacteristics(ctx, machineUUID)
		if errors.Is(err, machineerrors.MachineNotFound) {
			return nil, errors.NotFoundf("machine %q", m.Id())
		} else if err != nil && !errors.Is(err, machineerrors.NotProvisioned) {
			return nil, errors.Trace(err)
		}
		if hw != nil && hw.String() != "" {
			hwParams := &params.MachineHardware{
				Cores:            hw.CpuCores,
				Arch:             hw.Arch,
				Mem:              hw.Mem,
				RootDisk:         hw.RootDisk,
				CpuPower:         hw.CpuPower,
				Tags:             hw.Tags,
				AvailabilityZone: hw.AvailabilityZone,
				VirtType:         hw.VirtType,
			}
			mInfo.Hardware = hwParams
		}
		machineInfo = append(machineInfo, mInfo)
	}
	return machineInfo, nil
}
