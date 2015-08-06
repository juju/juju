// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package addresser

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names"

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
	*common.LifeGetter
	*common.Remover

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
	getAuthFunc := func() (common.AuthFunc, error) {
		return func(tag names.Tag) bool {
			return isEnvironManager
		}, nil
	}
	sti := getState(st)
	return &AddresserAPI{
		EnvironWatcher: common.NewEnvironWatcher(sti, resources, authorizer),
		LifeGetter:     common.NewLifeGetter(sti, getAuthFunc),
		Remover:        common.NewRemover(sti, false, getAuthFunc),
		st:             sti,
		resources:      resources,
		authorizer:     authorizer,
	}, nil
}

// ReleaseIPAddresses releases the IP addresses identified by the passed tags.
func (api *AddresserAPI) ReleaseIPAddresses(args params.Entities) (params.ErrorResults, error) {
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Entities)),
	}
	if len(args.Entities) == 0 {
		return result, nil
	}
	// Create an environment to verify networking support.
	config, err := api.st.EnvironConfig()
	if err != nil {
		return result, errors.Trace(err)
	}
	env, err := environs.New(config)
	if err != nil {
		return result, errors.Trace(err)
	}
	netEnv, ok := environs.SupportsNetworking(env)
	if !ok {
		return result, errors.NotSupportedf("IP address deallocation")
	}
	// Release all received IP addresses.
	for i, entity := range args.Entities {
		// Retrieve IP address by tag.
		tag, err := names.ParseIPAddressTag(entity.Tag)
		if err != nil {
			result.Results[i].Error = common.ServerError(err)
			continue
		}
		ipAddress, err := api.st.IPAddressByTag(tag)
		if err != nil {
			result.Results[i].Error = common.ServerError(err)
			continue
		}
		err = api.releaseIPAddress(netEnv, ipAddress)
		if err != nil {
			// Received invalid or non-dead entities or releasing on environment
			// is broken, so try again later.
			logger.Warningf("cannot release IP address %q: %v (will retry)", ipAddress.Value(), err)
			result.Results[i].Error = common.ServerError(common.ErrTryAgain)
		}
	}
	return result, nil
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
			watch.Stop()
			return params.EntityWatchResult{}, errors.Trace(err)
		}
		id := api.resources.Register(watch)
		return params.EntityWatchResult{
			EntityWatcherId: id,
			Changes:         mappedChanges,
		}, nil
	}
	return params.EntityWatchResult{}, watcher.EnsureErr(watch)
}
