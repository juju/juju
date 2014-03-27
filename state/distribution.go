// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"

	"launchpad.net/juju-core/errors"
	"launchpad.net/juju-core/instance"
)

// distribute takes a unit and set of clean, empty instances and asks the
// InstanceDistributor policy (if any) which ones are suitable for assigning
// the unit to. If there is no InstanceDistributor, or the distribution group
// is empty, then all of the candidates will be returned.
func (u *Unit) distribute(candidates []instance.Id) ([]instance.Id, error) {
	if u.st.policy == nil {
		return candidates, nil
	}
	cfg, err := u.st.EnvironConfig()
	if err != nil {
		return nil, err
	}
	distributor, err := u.st.policy.InstanceDistributor(cfg)
	if errors.IsNotImplementedError(err) {
		return candidates, nil
	} else if err != nil {
		return nil, err
	}
	if distributor == nil {
		return nil, fmt.Errorf("policy returned nil instance distributor without an error")
	}
	service, err := u.Service()
	if err != nil {
		return nil, err
	}
	distributionGroup, err := service.ServiceInstances()
	if err != nil {
		return nil, err
	}
	if len(distributionGroup) == 0 {
		return candidates, nil
	}
	return distributor.DistributeInstances(candidates, distributionGroup)
}

// ServiceInstances returns the instance IDs of provisioned
// machines that are assigned units of this service.
func (service *Service) ServiceInstances() ([]instance.Id, error) {
	units, err := service.AllUnits()
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
		machine, err := service.st.Machine(machineId)
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
