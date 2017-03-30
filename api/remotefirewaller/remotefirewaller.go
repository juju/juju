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

// WatchIngressAddressesForRelation returns a watcher that notifies when address from which
// connections will originate for the relation change.
func (c *Client) WatchIngressAddressesForRelation(remoteRelationId params.RemoteEntityId) (watcher.NotifyWatcher, error) {
	args := params.RemoteEntities{[]params.RemoteEntityId{remoteRelationId}}
	var result params.NotifyWatchResult
	err := c.facade.FacadeCall("WatchIngressAddressesForRelation", args, &result)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if result.Error != nil {
		return nil, errors.Trace(result.Error)
	}
	w := apiwatcher.NewNotifyWatcher(c.facade.RawAPICaller(), result)
	return w, nil
}

// IngressSubnetsForRelation returns any CIDRs for which ingress is required to allow
// the specified relation to properly function.
func (c *Client) IngressSubnetsForRelation(remoteRelationId params.RemoteEntityId) (*params.IngressSubnetInfo, error) {
	args := params.RemoteEntities{[]params.RemoteEntityId{remoteRelationId}}
	var results params.IngressSubnetResults
	err := c.facade.FacadeCall("IngressSubnetsForRelations", args, &results)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if len(results.Results) != 1 {
		return nil, errors.Errorf("expected %d result(s), got %d", 1, len(results.Results))
	}
	if err := results.Results[0].Error; err != nil {
		return nil, err
	}
	return results.Results[0].Result, nil
}
