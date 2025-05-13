// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machinemanager

import (
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/api/base"
)

func TestPackage(t *testing.T) {
	tc.TestingT(t)
}

func NewClientFromCaller(caller base.FacadeCaller) *Client {
	return &Client{
		facade: caller,
	}
}
