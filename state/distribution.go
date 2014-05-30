// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"

	"github.com/juju/errors"

	"launchpad.net/juju-core/instance"
)

// distributeuUnit takes a unit and set of clean, possibly empty, instances
// and asks the InstanceDistributor policy (if any) which ones are suitable
// for assigning the unit to. If there is no InstanceDistributor, or the
// distribution group is empty, then all of the candidates will be returned.
func distributeUnit(u *Unit, candidates []instance.Id) ([]instance.Id, error) {
	if len(candidates) == 0 {
		return nil, nil
	}
	if u.st.policy == nil {
		return candidates, nil
	}
	cfg, err := u.st.EnvironConfig()
	if err != nil {
		return nil, err
	}
	distributor, err := u.st.policy.InstanceDistributor(cfg)
	if errors.IsNotImplemented(err) {
		return candidates, nil
	} else if err != nil {
		return nil, err
	}
	if distributor == nil {
		return nil, fmt.Errorf("policy returned nil instance distributor without an error")
	}
	distributionGroup, err := ServiceInstances(u.st, u.doc.Service)
	if err != nil {
		return nil, err
	}
	if len(distributionGroup) == 0 {
		return candidates, nil
	}
	return distributor.DistributeInstances(candidates, distributionGroup)
}

// ServiceInstances returns the instance IDs of provisioned
// machines that are assigned units of the specified service.
func ServiceInstances(st *State, service string) ([]instance.Id, error) {
	units, err := allUnits(st, service)
	if err != nil {
		return nil, err
	}
	instanceIds := make([]instance.Id, 0, len(units))
	for _, unit := range units {
		machineId, err := unit.AssignedMachineId()
		if IsNotAssigned(err) {
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
		} else if IsNotProvisionedError(err) {
			continue
		} else {
			return nil, err
		}
	}
	return instanceIds, nil
}
