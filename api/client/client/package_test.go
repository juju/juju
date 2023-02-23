// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client

import (
	"testing"

	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/base"
)

func TestAll(t *testing.T) {
	gc.TestingT(t)
}

func NewClientFromFacadeCaller(facade base.FacadeCaller) *Client {
	return &Client{
		facade: facade,
	}
}
