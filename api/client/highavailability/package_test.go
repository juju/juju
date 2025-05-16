// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package highavailability

import (
	stdtesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/api/base"
)

func TestAll(t *stdtesting.T) {
	tc.TestingT(t)
}

func NewClientFromCaller(caller base.FacadeCaller, facade base.ClientFacade) *Client {
	return &Client{
		ClientFacade: facade,
		facade:       caller,
	}
}
