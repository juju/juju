// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package addresser

import (
	"strings"

	"github.com/juju/errors"
	"github.com/juju/loggo"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/watcher"
)

func init() {
	common.RegisterStandardFacade("Addresser", 1, NewAddresserAPI)
}

var logger = loggo.GetLogger("juju.apiserver.addresser")

// AddresserAPI provides access to the Addresser API facade.
type AddresserAPI struct {
	*common.EnvironWatcher

	st         StateInterface
	resources  *common.Resources
	authorizer common.Authorizer
}

// NewAddresserAPI creates a new server-side Addresser API facade.
func NewAddresserAPI(
	st *state.State,
	resources *common.Resources,
	authorizer common.Authorizer,
) (*AddresserAPI, error) {
	isEnvironManager := authorizer.AuthEnvironManager()
	if !isEnvironManager {
		// Addresser must run as environment manager.
		return nil, common.ErrPerm
	}
	sti := getState(st)
	return &AddresserAPI{
		EnvironWatcher: common.NewEnvironWatcher(sti, resources, authorizer),
		st:             sti,
		resources:      resources,
		authorizer:     authorizer,
	}, nil
}

// CleanupIPAddresses releases and removes the dead IP addresses.
func (api *AddresserAPI) CleanupIPAddresses() error {
	// Create an environment to verify networking support.
	config, err := api.st.EnvironConfig()
	if err != nil {
		return errors.Trace(err)
	}
	env, err := environs.New(config)
	if err != nil {
		return errors.Trace(err)
	}
	netEnv, ok := environs.SupportsNetworking(env)
	if !ok {
		return errors.NotSupportedf("IP address deallocation")
	}
	// Retrieve dead addresses, release and remove them.
	logger.Debugf("retrieving dead IP addresses")
	ipAddresses, err := api.st.DeadIPAddresses()
	if err != nil {
		return errors.Trace(err)
	}
	var retryIPAddresses []StateIPAddress
	for _, ipAddress := range ipAddresses {
		logger.Debugf("releasing dead IP address %q", ipAddress.Value())
		err := api.releaseIPAddress(netEnv, ipAddress)
		if err != nil {
			logger.Warningf("cannot release IP address %q: %v (will retry)", ipAddress.Value(), err)
			retryIPAddresses = append(retryIPAddresses, ipAddress)
			continue
		}
		logger.Debugf("removing released IP address %q", ipAddress.Value())
		err = ipAddress.Remove()
		if err != nil {
			return errors.Trace(err)
		}
	}
	if len(retryIPAddresses) > 0 {
		values := make([]string, len(retryIPAddresses))
		for i, ipAddress := range retryIPAddresses {
			values[i] = ipAddress.Value()
		}
		return errors.Errorf("some addresses not released (will retry): %v", strings.Join(values, " / "))
	}
	return nil
}

// releaseIPAddress releases one IP address.
func (api *AddresserAPI) releaseIPAddress(netEnv environs.NetworkingEnviron, ipAddress StateIPAddress) (err error) {
	defer errors.DeferredAnnotatef(&err, "failed to release IP address %q", ipAddress.Value())
	logger.Tracef("attempting to release dead IP address %q", ipAddress.Value())
	// Final check if IP address is really dead.
	if ipAddress.Life() != state.Dead {
		return errors.New("IP address not dead")
	}
	// Now release the IP address.
	subnetId := network.Id(ipAddress.SubnetId())
	err = netEnv.ReleaseAddress(ipAddress.InstanceId(), subnetId, ipAddress.Address(), ipAddress.MACAddress())
	if err != nil {
		return errors.Trace(err)
	}
	return nil
}

// WatchIPAddresses observes changes to the IP addresses.
func (api *AddresserAPI) WatchIPAddresses() (params.EntityWatchResult, error) {
	watch := &ipAddressesWatcher{api.st.WatchIPAddresses(), api.st}

	if changes, ok := <-watch.Changes(); ok {
		mappedChanges, err := watch.MapChanges(changes)
		if err != nil {
			return params.EntityWatchResult{}, errors.Trace(err)
		}
		return params.EntityWatchResult{
			EntityWatcherId: api.resources.Register(watch),
			Changes:         mappedChanges,
		}, nil
	}
	return params.EntityWatchResult{}, watcher.EnsureErr(watch)
}
