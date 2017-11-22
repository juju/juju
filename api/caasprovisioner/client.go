// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasprovisioner

import (
	"github.com/juju/juju/api/base"
	apiwatcher "github.com/juju/juju/api/watcher"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/watcher"
)

// Client allows access to the CAAS provisioner API endpoint.
type Client struct {
	facade base.FacadeCaller
}

// NewClient returns a client used to access the CAAS Provisioner API.
func NewClient(caller base.APICaller) *Client {
	facadeCaller := base.NewFacadeCaller(caller, "CAASProvisioner")
	return &Client{
		facade: facadeCaller,
	}
}

// WatchApplications returns a StringsWatcher that notifies of
// changes to the lifecycles of CAAS applications in the current model.
func (st *Client) WatchApplications() (watcher.StringsWatcher, error) {
	var result params.StringsWatchResult
	if err := st.facade.FacadeCall("WatchApplications", nil, &result); err != nil {
		return nil, err
	}
	if err := result.Error; err != nil {
		return nil, result.Error
	}
	w := apiwatcher.NewStringsWatcher(st.facade.RawAPICaller(), result)
	return w, nil
}
