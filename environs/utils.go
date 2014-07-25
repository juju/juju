package environs

import (
	"fmt"
	"time"

	"github.com/juju/errors"
	"github.com/juju/utils"

	"github.com/juju/juju/environs/storage"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/api"
)

// GetStorage creates an Environ from the config in state and returns
// its storage interface.
func GetStorage(st *state.State) (storage.Storage, error) {
	envConfig, err := st.EnvironConfig()
	if err != nil {
		return nil, fmt.Errorf("cannot get environment config: %v", err)
	}
	env, err := New(envConfig)
	if err != nil {
		return nil, fmt.Errorf("cannot access environment: %v", err)
	}
	return env.Storage(), nil
}

var longAttempt = utils.AttemptStrategy{
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
	env Environ,
	instanceIds []instance.Id,
) ([]network.Address, error) {
	var addrs []network.Address
	for a := longAttempt.Start(); len(addrs) == 0 && a.Next(); {
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
func APIInfo(env Environ) (*api.Info, error) {
	instanceIds, err := env.StateServerInstances()
	if err != nil {
		return nil, err
	}
	logger.Debugf("StateServerInstances returned: %v", instanceIds)
	addrs, err := waitAnyInstanceAddresses(env, instanceIds)
	if err != nil {
		return nil, err
	}
	config := env.Config()
	cert, hasCert := config.CACert()
	if !hasCert {
		return nil, errors.New("config has no CACert")
	}
	apiPort := config.APIPort()
	apiAddrs := make([]string, len(addrs))
	for i, hp := range network.AddressesWithPort(addrs, apiPort) {
		apiAddrs[i] = hp.NetAddr()
	}
	apiInfo := &api.Info{Addrs: apiAddrs, CACert: cert}
	return apiInfo, nil
}
