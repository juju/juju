// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package addresser

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/instance"
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
	st            StateInterface
	resources     *common.Resources
	authorizer    common.Authorizer
	netEnv        environs.NetworkingEnviron
	canDeallocate bool
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
		st:         sti,
		resources:  resources,
		authorizer: authorizer,
	}, nil
}

// CanDeallocateAddresses checks if the current environment can
// deallocate IP addresses.
func (api *AddresserAPI) CanDeallocateAddresses() params.BoolResult {
	result := params.BoolResult{}
	// Create an environment to verify networking support.
	config, err := api.st.EnvironConfig()
	if err != nil {
		err = errors.Annotate(err, "getting environment config")
		result.Error = common.ServerError(err)
		return result
	}
	env, err := environs.New(config)
	if err != nil {
		err = errors.Annotate(err, "validating environment config")
		result.Error = common.ServerError(err)
		return result
	}
	netEnv, ok := environs.SupportsNetworking(env)
	if !ok {
		result.Error = common.ServerError(errors.NotSupportedf("IP address deallocation"))
		return result
	}
	api.netEnv = netEnv
	api.canDeallocate, err = api.netEnv.SupportsAddressAllocation(network.AnySubnet)
	if err != nil {
		err = errors.Annotate(err, "checking allocation support")
		result.Error = common.ServerError(err)
		return result
	}
	result.Result = api.canDeallocate
	return result
}

// CleanupIPAddresses releases and removes the dead IP addresses.
func (api *AddresserAPI) CleanupIPAddresses() params.ErrorResult {
	result := params.ErrorResult{}
	// Lazy setting of the networking environment, so only
	// has to be done once.
	if api.netEnv == nil {
		checkResult := api.CanDeallocateAddresses()
		if checkResult.Error != nil {
			result.Error = checkResult.Error
			return result
		}
	}
	// Check flag set inside of CanDeallocateAddresses.
	if !api.canDeallocate {
		result.Error = common.ServerError(errors.NotSupportedf("IP address deallocation"))
		return result
	}
	// Retrieve dead addresses, release and remove them.
	logger.Debugf("retrieving dead IP addresses")
	ipAddresses, err := api.st.DeadIPAddresses()
	if err != nil {
		err = errors.Annotate(err, "getting dead addresses")
		result.Error = common.ServerError(err)
		return result
	}
	canRetry := false
	for _, ipAddress := range ipAddresses {
		ipAddressValue := ipAddress.Value()
		logger.Debugf("releasing dead IP address %q", ipAddressValue)
		err := api.releaseIPAddress(api.netEnv, ipAddress)
		if err != nil {
			logger.Warningf("cannot release IP address %q: %v (will retry)", ipAddressValue, err)
			canRetry = true
			continue
		}
		logger.Debugf("removing released IP address %q", ipAddressValue)
		err = ipAddress.Remove()
		if errors.IsNotFound(err) {
			continue
		}
		if err != nil {
			logger.Warningf("failed to remove released IP address %q: %v (will retry)", ipAddressValue, err)
			canRetry = true
			continue
		}
	}
	if canRetry {
		result.Error = common.ServerError(common.ErrTryAgain)
	}
	return result
}

// netEnvReleaseAddress is used for testability.
var netEnvReleaseAddress = func(env environs.NetworkingEnviron,
	instId instance.Id, subnetId network.Id, addr network.Address, macAddress string) error {
	return env.ReleaseAddress(instId, subnetId, addr, macAddress)
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
	err = netEnvReleaseAddress(netEnv, ipAddress.InstanceId(), subnetId, ipAddress.Address(), ipAddress.MACAddress())
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
