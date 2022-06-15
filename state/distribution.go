// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"

	"github.com/juju/collections/set"
	"github.com/juju/errors"

	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/environs/context"
)

// distributeUnit takes a unit and set of clean, possibly empty, instances
// and asks the InstanceDistributor policy (if any) which ones are suitable
// for assigning the unit to. If there is no InstanceDistributor, or the
// distribution group is empty, then all of the candidates will be returned.
func distributeUnit(u *Unit, candidates []instance.Id, limitZones []string) ([]instance.Id, error) {
	if len(candidates) == 0 {
		return nil, nil
	}
	if u.st.policy == nil {
		return candidates, nil
	}

	distributor, err := u.st.policy.InstanceDistributor()
	if errors.IsNotImplemented(err) {
		return candidates, nil
	} else if err != nil {
		return nil, err
	}
	if distributor == nil {
		return nil, fmt.Errorf("policy returned nil instance distributor without an error")
	}

	distributionGroup, err := ApplicationInstances(u.st, u.doc.Application)
	if err != nil {
		return nil, err
	}
	if len(distributionGroup) == 0 {
		return candidates, nil
	}
	return distributor.DistributeInstances(context.CallContext(u.st), candidates, distributionGroup, limitZones)
}

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
		if errors.IsNotAssigned(err) {
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
		} else if errors.IsNotProvisioned(err) {
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
