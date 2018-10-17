// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charms

import "github.com/juju/juju/api/base"

func NewClientWithFacade(facade base.FacadeCaller) *Client {
	return &Client{facade: facade}
}
