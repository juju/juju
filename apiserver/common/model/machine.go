// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"context"

	"github.com/juju/errors"

	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/machine"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

// ModelMachineInfo returns information about machine hardware for
// alive top level machines (not containers).
func ModelMachineInfo(ctx context.Context, st ModelManagerBackend, machineService MachineService) (machineInfo []params.ModelMachineInfo, _ error) {
	machines, err := st.AllMachines()
	if err != nil {
		return nil, errors.Trace(err)
	}
	controllerNodes, err := st.ControllerNodes()
	if err != nil {
		return nil, errors.Trace(err)
	}
	hasVote := make(map[string]bool)
	wantsVote := make(map[string]bool)
	for _, n := range controllerNodes {
		hasVote[n.Id()] = n.HasVote()
		wantsVote[n.Id()] = n.WantsVote()
	}
	var primaryID string
	primaryHA, err := st.HAPrimaryMachine()
	if err != nil {
		// We do not want to return any errors here as they are all
		// non-fatal for this call since we can still
		// get machine info even if we could not get HA Primary determined.
		// Also on some non-HA setups, i.e. where mongo was not run with --replSet,
		// this call will return an error.
		logger.Warningf(ctx, "could not determine if there is a primary HA machine: %v", err)
	}
	if len(controllerNodes) > 1 {
		primaryID = primaryHA.Id()
	}

	for _, m := range machines {
		if m.Life() != state.Alive {
			continue
		}
		var aStatus string
		// This is suboptimal as if there are many machines,
		// we are making many calls into the DB for each machine.
		var statusMessage string
		statusInfo, err := m.Status()
		if err == nil {
			aStatus = string(statusInfo.Status)
			statusMessage = statusInfo.Message
		} else {
			aStatus = err.Error()
		}
		mInfo := params.ModelMachineInfo{
			Id:        m.Id(),
			HasVote:   hasVote[m.Id()],
			WantsVote: wantsVote[m.Id()],
			Status:    aStatus,
			Message:   statusMessage,
		}
		if primaryID != "" {
			if isPrimary := primaryID == m.Id(); isPrimary {
				mInfo.HAPrimary = &isPrimary
			}
		}
		machineUUID, err := machineService.GetMachineUUID(ctx, machine.Name(m.Id()))
		if errors.Is(err, machineerrors.MachineNotFound) {
			return nil, errors.NotFoundf("machine %q", m.Id())
		} else if err != nil {
			return nil, errors.Trace(err)
		}
		instanceID, displayName, err := machineService.InstanceIDAndName(ctx, machineUUID)
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
		hw, err := machineService.HardwareCharacteristics(ctx, machineUUID)
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
