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

// WatchIngressAddressesForRelation returns a watcher that notifies when address, from which
// connections will originate for the relation, change.
// Each event contains the entire set of addresses which are required for ingress for the relation.
func (c *Client) WatchIngressAddressesForRelation(remoteRelationId params.RemoteEntityId) (watcher.StringsWatcher, error) {
	args := params.RemoteEntities{[]params.RemoteEntityId{remoteRelationId}}
	var results params.StringsWatchResults
	err := c.facade.FacadeCall("WatchIngressAddressesForRelation", args, &results)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if len(results.Results) != 1 {
		return nil, errors.Errorf("expected 1 result, got %d", len(results.Results))
	}
	result := results.Results[0]
	if result.Error != nil {
		return nil, result.Error
	}
	w := apiwatcher.NewStringsWatcher(c.facade.RawAPICaller(), result)
	return w, nil
}
