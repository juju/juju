// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// charms provides a client for accessing the charms API.
package charms

import (
	"github.com/juju/errors"

	"github.com/juju/juju/api/base"
	commoncharms "github.com/juju/juju/api/common/charms"
	"github.com/juju/juju/apiserver/params"
)

// Client allows access to the charms API end point.
type Client struct {
	base.ClientFacade
	*commoncharms.CharmsClient
	facade base.FacadeCaller
}

// NewClient creates a new client for accessing the charms API.
func NewClient(st base.APICallCloser) *Client {
	frontend, backend := base.NewClientFacade(st, "Charms")
	commonClient := commoncharms.NewCharmsClient(backend)
	return &Client{ClientFacade: frontend, CharmsClient: commonClient, facade: backend}
}

// IsMetered returns whether or not the charm is metered.
func (c *Client) IsMetered(charmURL string) (bool, error) {
	args := params.CharmURL{URL: charmURL}
	var metered params.IsMeteredResult
	if err := c.facade.FacadeCall("IsMetered", args, &metered); err != nil {
		return false, errors.Trace(err)
	}
	return metered.Metered, nil
}
