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
	providercommon "github.com/juju/juju/provider/common"
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
		return result, err
	}
	env, err := environs.New(config)
	if err != nil {
		return result, err
	}
	if netEnv, ok := environs.SupportsNetworking(env); ok {
		logger.Debugf("environment supports networking")
		for i, entity := range args.Entities {
			tag, err := names.ParseTag(entity.Tag)
			if err != nil {
				result.Results[i].Error = common.ServerError(err)
				continue
			}
			err = api.releaseIPAddress(netEnv, tag)
			if err != nil {
				result.Results[i].Error = common.ServerError(err)
			}
		}
	}
	return result, nil
}

// releaseIPAddress releases one IP address.
func (api *AddresserAPI) releaseIPAddress(netEnv environs.NetworkingEnviron, tag names.Tag) (err error) {
	defer errors.DeferredAnnotatef(&err, "failed to release IP address %v", tag.String())
	logger.Debugf("attempting to release dead IP address %v", tag.String())
	// Retrieve IP address by tag.
	ipAddress, err := api.st.IPAddressByTag(tag.(names.IPAddressTag))
	if err != nil {
		return errors.Trace(err)
	}
	// Try to release it.
	subnetId := network.Id(ipAddress.SubnetId())
	for attempt := providercommon.ShortAttempt.Start(); attempt.Next(); {
		if err = netEnv.ReleaseAddress(ipAddress.InstanceId(),
			subnetId, ipAddress.Address(), ipAddress.MACAddress()); err == nil {
			return nil
		}
	}
	// Don't remove the address from state so we
	// can retry releasing the address later.
	logger.Warningf("cannot release address %q: %v (will retry)", tag, err)
	return errors.Trace(err)
}

// WatchIPAddresses observes changes to the IP addresses.
func (api *AddresserAPI) WatchIPAddresses() (params.EntityWatchResult, error) {
	watch := &ipAddressesWatcher{api.st.WatchIPAddresses(), api.st}

	if changes, ok := <-watch.Changes(); ok {
		mappedChanges, err := watch.MapChanges(changes)
		if err != nil {
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
