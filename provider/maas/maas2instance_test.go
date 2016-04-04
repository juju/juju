// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package maas

import (
	gc "gopkg.in/check.v1"
)

type maas2InstanceSuite struct {
	baseProviderSuite
}

var _ = gc.Suite(&maas2InstanceSuite{})

func (s *maas2InstanceSuite) TestString(c *gc.C) {
	instance := &maas2Instance{&fakeMachine{hostname: "peewee", systemID: "herman"}}
	c.Assert(instance.String(), gc.Equals, "peewee:herman")
}
