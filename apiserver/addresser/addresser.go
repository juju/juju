// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package addresser

import (
	"fmt"

	"github.com/juju/loggo"
	"github.com/juju/names"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/watcher"
)

func init() {
	common.RegisterStandardFacade("Addresser", 1, NewAddresserAPI)
}

var logger = loggo.GetLogger("juju.apiserver.addresser")

// AddresserAPI provides access to the Addresser API facade.
type AddresserAPI struct {
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
		}
	}
	sti := getState(st)
	return &AddresserAPI{
		LifeGetter: common.NewLifeGetter(sti, getAuthFunc),
		Remover:    common.NewRemover(sti, false, getAuthFunc),
		st:         sti,
		resources:  resources,
		authorizer: authorizer,
	}, nil
}

// WatchIPAddresses observes changes to the IP addresses.
func (a *AddresserAPI) WatchIPAddresses() (params.StringsWatchResult, error) {
	result := params.StringsWatchResult{}
	if !a.authorizer.AuthEnvironManager() {
		return result, common.ErrPerm
	}
	watch := a.st.WatchIPAddresses()
	// Consume the initial event and forward it to the result.
	if changes, ok := <-watch.Changes(); ok {
		result.StringsWatcherId = a.resources.Register(watch)
		result.Changes = changes
	} else {
		err := watcher.EnsureErr(watch)
		return result, fmt.Errorf("cannot obtain initial IP addresses: %v", err)
	}
	return result, nil
}
