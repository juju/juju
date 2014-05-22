// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"errors"
	"fmt"

	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/bootstrap"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api"
)

// getAddresses queries and returns the Addresses for the given instances,
// ignoring nil instances or ones without addresses.
func getAddresses(instances []instance.Instance) []string {
	names := make([]string, 0)
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
		for _, addr := range addrs {
			names = append(names, addr.Value)
		}
	}
	return names
}

// composeAddresses suffixes each of a slice of hostnames with a given port
// number.
func composeAddresses(hostnames []string, port int) []string {
	addresses := make([]string, len(hostnames))
	for index, hostname := range hostnames {
		addresses[index] = fmt.Sprintf("%s:%d", hostname, port)
	}
	return addresses
}

// getStateInfo puts together the state.Info and api.Info for the given
// config, with the given state-server host names.
// The given config absolutely must have a CACert.
func getStateInfo(config *config.Config, hostnames []string) (*state.Info, *api.Info) {
	cert, hasCert := config.CACert()
	if !hasCert {
		panic(errors.New("getStateInfo: config has no CACert"))
	}
	return &state.Info{
			Addrs:  composeAddresses(hostnames, config.StatePort()),
			CACert: cert,
		}, &api.Info{
			Addrs:  composeAddresses(hostnames, config.APIPort()),
			CACert: cert,
		}
}

// StateInfo is a reusable implementation of Environ.StateInfo, available to
// providers that also use the other functionality from this file.
func StateInfo(env environs.Environ) (*state.Info, *api.Info, error) {
	st, err := bootstrap.LoadState(env.Storage())
	if err != nil {
		return nil, nil, err
	}
	config := env.Config()
	if _, hasCert := config.CACert(); !hasCert {
		return nil, nil, fmt.Errorf("no CA certificate in environment configuration")
	}
	// Wait for the addresses of at least one of the instances to become available.
	logger.Debugf("waiting for addresses of state server instances %v", st.StateInstances)
	var addresses []string
	for a := LongAttempt.Start(); len(addresses) == 0 && a.Next(); {
		insts, err := env.Instances(st.StateInstances)
		if err != nil && err != environs.ErrPartialInstances {
			logger.Debugf("error getting state instances: %v", err.Error())
			return nil, nil, err
		}
		addresses = getAddresses(insts)
	}

	if len(addresses) == 0 {
		return nil, nil, fmt.Errorf("timed out waiting for addresses from %v", st.StateInstances)
	}

	stateInfo, apiInfo := getStateInfo(config, addresses)
	return stateInfo, apiInfo, nil
}
