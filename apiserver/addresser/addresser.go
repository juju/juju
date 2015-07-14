// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package addresser

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/watcher"
)

func init() {
	// TODO(mue) Remove comment when client is implemented.
	// common.RegisterStandardFacade("Addresser", 1, NewAddresserAPI)
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

// WatchIPAddresses observes changes to the IP addresses.
func (api *AddresserAPI) WatchIPAddresses() (params.EntityWatchResult, error) {
	watch := &ipAddressesWatcher{api.st.WatchIPAddresses()}

	if changes, ok := <-watch.Changes(); ok {
		mappedChanges, err := watch.MapChanges(api.st, changes)
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
