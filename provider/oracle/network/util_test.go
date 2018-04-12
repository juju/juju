// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/provider/oracle/network"
)

type utilSuite struct{}

var _ = gc.Suite(&utilSuite{})

func (u *utilSuite) TestGetMacAndIP(c *gc.C) {
	macAddress := "29-E7-9F-D9-83-C8"
	ipAddress := "120.120.120.0"
	address := []string{
		macAddress,
		ipAddress,
	}

	mac, ip, err := network.GetMacAndIP(address)
	c.Assert(err, gc.IsNil)
	c.Assert(mac, jc.DeepEquals, macAddress)
	c.Assert(ip, jc.DeepEquals, ipAddress)
}

func (u *utilSuite) TestGetMacAndIPWithError(c *gc.C) {
	macAddress1 := "29-E7-9F-D9-83-C8"
	macAddress2 := "UDIUSHIDUSIDHSIUDh"
	ipAddress1 := "idaijds.daisdia.daishd"
	ipAddress2 := "127.0.0.1"

	address := []string{
		macAddress1,
		ipAddress1,
		macAddress2,
		ipAddress2,
	}

	mac, ip, err := network.GetMacAndIP(address)
	c.Assert(err, gc.NotNil)
	c.Assert(mac, jc.DeepEquals, macAddress1)
	c.Assert(ip, jc.DeepEquals, "")

	mac, ip, err = network.GetMacAndIP(nil)
	c.Assert(err, gc.NotNil)
	c.Assert(mac, jc.DeepEquals, "")
	c.Assert(ip, jc.DeepEquals, "")

	address = []string{
		ipAddress2,
		ipAddress1,
	}

	mac, ip, err = network.GetMacAndIP(address)
	c.Assert(err, gc.NotNil)
	c.Assert(mac, jc.DeepEquals, "")
	c.Assert(ip, jc.DeepEquals, ipAddress2)
}
