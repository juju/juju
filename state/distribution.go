// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/collections/set"
	"github.com/juju/errors"

	"github.com/juju/juju/core/instance"
)

// ApplicationInstances returns the instance IDs of provisioned
// machines that are assigned units of the specified application.
func ApplicationInstances(st *State, application string) ([]instance.Id, error) {
	units, err := allUnits(st, application)
	if err != nil {
		return nil, err
	}
	instanceIds := make([]instance.Id, 0, len(units))
	for _, unit := range units {
		machineId, err := unit.AssignedMachineId()
		if errors.Is(err, errors.NotAssigned) {
			continue
		} else if err != nil {
			return nil, err
		}
		machine, err := st.Machine(machineId)
		if err != nil {
			return nil, err
		}
		instanceId, err := machine.InstanceId()
		if err == nil {
			instanceIds = append(instanceIds, instanceId)
		} else if errors.Is(err, errors.NotProvisioned) {
			continue
		} else {
			return nil, err
		}
	}
	return instanceIds, nil
}

// ApplicationMachines returns the machine IDs of machines which have
// the specified application listed as a principal.
func ApplicationMachines(st *State, application string) ([]string, error) {
	machines, err := st.AllMachines()
	if err != nil {
		return nil, err
	}
	applicationName := unitAppName(application)
	var machineIds []string
	for _, machine := range machines {
		principalSet := set.NewStrings()
		for _, principal := range machine.Principals() {
			principalSet.Add(unitAppName(principal))
		}
		if principalSet.Contains(applicationName) {
			machineIds = append(machineIds, machine.Id())
		}
	}
	return machineIds, nil
}
