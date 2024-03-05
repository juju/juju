// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package highavailability

import (
	"testing"

	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/base"
)

func TestAll(t *testing.T) {
	gc.TestingT(t)
}

func NewClientFromCaller(caller base.FacadeCaller, facade base.ClientFacade) *Client {
	return &Client{
		ClientFacade: facade,
		facade:       caller,
	}
}
