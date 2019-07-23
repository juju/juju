// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network_test

import (
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/network"
	"github.com/juju/juju/testing"
)

type NetworkSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&NetworkSuite{})

func (s *NetworkSuite) TestGenerateVirtualMACAddress(c *gc.C) {
	mac := network.GenerateVirtualMACAddress()
	c.Check(mac, gc.Matches, "^([0-9A-Fa-f]{2}[:-]){5}([0-9A-Fa-f]{2})$")
}
