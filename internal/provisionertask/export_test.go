// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisionertask

import (
	"context"
	"sort"

	apiprovisioner "github.com/juju/juju/api/agent/provisioner"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/rpc/params"
)

var ClassifyMachine = classifyMachine

// GetCopyAvailabilityZoneMachines returns a copy of p.(*provisionerTask).availabilityZoneMachines
func GetCopyAvailabilityZoneMachines(p ProvisionerTask) []AvailabilityZoneMachine {
	task := p.(*provisionerTask)
	task.machinesMutex.RLock()
	defer task.machinesMutex.RUnlock()
	// Sort to make comparisons in the tests easier.
	zoneMachines := task.availabilityZoneMachines
	sort.Slice(task.availabilityZoneMachines, func(i, j int) bool {
		switch {
		case zoneMachines[i].MachineIds.Size() < zoneMachines[j].MachineIds.Size():
			return true
		case zoneMachines[i].MachineIds.Size() == zoneMachines[j].MachineIds.Size():
			return zoneMachines[i].ZoneName < zoneMachines[j].ZoneName
		}
		return false
	})
	retvalues := make([]AvailabilityZoneMachine, len(zoneMachines))
	for i := range zoneMachines {
		retvalues[i] = *zoneMachines[i]
	}
	return retvalues
}

func SetupToStartMachine(
	p ProvisionerTask,
	machine apiprovisioner.MachineProvisioner,
	version *semversion.Number,
	pInfoResult params.ProvisioningInfoResult,
) (environs.StartInstanceParams, error) {
	return p.(*provisionerTask).setupToStartMachine(context.Background(), machine, version, pInfoResult)
}
