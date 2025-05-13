// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client

import (
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/api/base"
)

func TestAll(t *testing.T) {
	tc.TestingT(t)
}

func NewClientFromFacadeCaller(facade base.FacadeCaller) *Client {
	return &Client{
		facade: facade,
	}
}
