// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charms

import (
	"github.com/juju/juju/api/base"
	commoncharms "github.com/juju/juju/api/common/charms"
)

func NewClientWithFacade(facade base.FacadeCaller) *Client {
	charmInfoClient := commoncharms.NewCharmInfoClient(facade)
	return &Client{facade: facade, CharmInfoClient: charmInfoClient}
}
