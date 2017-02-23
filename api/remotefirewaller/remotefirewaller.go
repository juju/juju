// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package remotefirewaller

import (
	"github.com/juju/errors"

	"github.com/juju/juju/api/base"
	apiwatcher "github.com/juju/juju/api/watcher"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/watcher"
)

const remoteFirewallerFacade = "RemoteFirewaller"

// Client provides access to the networks api facade.
type Client struct {
	base.ClientFacade
	facade base.FacadeCaller
}

// NewClient creates a new client-side Networks facade.
func NewClient(caller base.APICallCloser) *Client {
	frontend, backend := base.NewClientFacade(caller, remoteFirewallerFacade)
	return &Client{ClientFacade: frontend, facade: backend}
}

// WatchSubnets returns a strings watcher that notifies of the addition,
// removal, and lifecycle changes of subnets in the model.
func (c *Client) WatchSubnets() (watcher.StringsWatcher, error) {
	var result params.StringsWatchResult
	err := c.facade.FacadeCall("WatchSubnets", nil, &result)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if result.Error != nil {
		return nil, errors.Trace(result.Error)
	}
	w := apiwatcher.NewStringsWatcher(c.facade.RawAPICaller(), result)
	return w, nil
}

func (c *Client) Close() error {
	return c.ClientFacade.Close()
}
