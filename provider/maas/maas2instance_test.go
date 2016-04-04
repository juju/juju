// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package maas

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
)

type maas2InstanceSuite struct {
	baseProviderSuite
}

var _ = gc.Suite(&maas2InstanceSuite{})

func (s *maas2InstanceSuite) TestString(c *gc.C) {
	instance := &maas2Instance{&fakeMachine{hostname: "peewee", systemID: "herman"}}
	c.Assert(instance.String(), gc.Equals, "peewee:herman")
}

func (s *maas2InstanceSuite) TestID(c *gc.C) {
	thing := &maas2Instance{&fakeMachine{systemID: "herman"}}
	c.Assert(thing.Id(), gc.Equals, instance.Id("herman"))
}

func (s *maas2InstanceSuite) TestAddresses(c *gc.C) {
	instance := &maas2Instance{&fakeMachine{ipAddresses: []string{
		"0.0.0.0",
		"1.2.3.4",
		"127.0.0.1",
	}}}
	expectedAddresses := []network.Address{
		network.NewAddress("0.0.0.0"),
		network.NewAddress("1.2.3.4"),
		network.NewAddress("127.0.0.1"),
	}
	addresses, err := instance.Addresses()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(addresses, jc.SameContents, expectedAddresses)
}

func (s *maas2InstanceSuite) TestZone(c *gc.C) {
	instance := &maas2Instance{&fakeMachine{zoneName: "inflatable"}}
	c.Assert(instance.zone(), gc.Equals, "inflatable")
}
