// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud

import (
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/api/base"
)

func TestAll(t *testing.T) {
	tc.TestingT(t)
}

func NewClientFromCaller(caller base.FacadeCaller) *Client {
	return &Client{
		facade: caller,
	}
}
