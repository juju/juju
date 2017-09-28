// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environs

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/utils"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/api"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
)

// AddressesRefreshAttempt is the attempt strategy used when
// refreshing instance addresses.
var AddressesRefreshAttempt = utils.AttemptStrategy{
	Total: 3 * time.Minute,
	Delay: 1 * time.Second,
}

// getAddresses queries and returns the Addresses for the given instances,
// ignoring nil instances or ones without addresses.
func getAddresses(instances []instance.Instance) []network.Address {
	var allAddrs []network.Address
	for _, inst := range instances {
		if inst == nil {
			continue
		}
		addrs, err := inst.Addresses()
		if err != nil {
			logger.Debugf(
				"failed to get addresses for %v: %v (ignoring)",
				inst.Id(), err,
			)
			continue
		}
		allAddrs = append(allAddrs, addrs...)
	}
	return allAddrs
}

// waitAnyInstanceAddresses waits for at least one of the instances
// to have addresses, and returns them.
func waitAnyInstanceAddresses(
	env IAASEnviron,
	instanceIds []instance.Id,
) ([]network.Address, error) {
	var addrs []network.Address
	for a := AddressesRefreshAttempt.Start(); len(addrs) == 0 && a.Next(); {
		instances, err := env.Instances(instanceIds)
		if err != nil && err != ErrPartialInstances {
			logger.Debugf("error getting state instances: %v", err)
			return nil, err
		}
		addrs = getAddresses(instances)
	}
	if len(addrs) == 0 {
		return nil, errors.NotFoundf("addresses for %v", instanceIds)
	}
	return addrs, nil
}

// APIInfo returns an api.Info for the environment. The result is populated
// with addresses and CA certificate, but no tag or password.
func APIInfo(controllerUUID, modelUUID, caCert string, apiPort int, env IAASEnviron) (*api.Info, error) {
	instanceIds, err := env.ControllerInstances(controllerUUID)
	if err != nil {
		return nil, err
	}
	logger.Debugf("ControllerInstances returned: %v", instanceIds)
	addrs, err := waitAnyInstanceAddresses(env, instanceIds)
	if err != nil {
		return nil, err
	}
	apiAddrs := network.HostPortsToStrings(
		network.AddressesWithPort(addrs, apiPort),
	)
	modelTag := names.NewModelTag(modelUUID)
	apiInfo := &api.Info{Addrs: apiAddrs, CACert: caCert, ModelTag: modelTag}
	return apiInfo, nil
}

// CheckProviderAPI returns an error if a simple API call
// to check a basic response from the specified environ fails.
func CheckProviderAPI(env Environ) error {

	ienv, ok := env.(IAASEnviron)
	if !ok {
		// non-IAAS environs do not support AllInstances
		// TODO(caas) find a real way to ping the substrate
		return nil
	}

	// We will make a simple API call to the provider
	// to ensure the underlying substrate is ok.
	_, err := ienv.AllInstances()
	switch err {
	case nil, ErrPartialInstances, ErrNoInstances:
		return nil
	}
	return errors.Annotate(err, "cannot make API call to provider")
}
