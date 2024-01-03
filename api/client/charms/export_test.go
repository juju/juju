// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charms

import (
	"github.com/juju/juju/api/base"
	commoncharms "github.com/juju/juju/api/common/charms"
)

var HasHooksOrDispatch = &hasHooksOrDispatch

func NewClientWithFacade(facade base.FacadeCaller, clientFacade base.ClientFacade) *Client {
	charmInfoClient := commoncharms.NewCharmInfoClient(facade)
	return &Client{facade: facade, ClientFacade: clientFacade, CharmInfoClient: charmInfoClient}
}

func NewLocalCharmClientWithFacade(facade base.FacadeCaller, clientFacade base.ClientFacade) *LocalCharmClient {
	return &LocalCharmClient{facade: facade, ClientFacade: clientFacade}
}
