// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package factory_test

import (
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/container"
	"launchpad.net/juju-core/container/factory"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/testing/testbase"
)

type factorySuite struct {
	testbase.LoggingSuite
}

var _ = gc.Suite(&factorySuite{})

func (*factorySuite) TestNewContainerManager(c *gc.C) {
	conf := container.ManagerConfig{Name: "test"}

	for _, test := range []struct {
		containerType instance.ContainerType
		valid         bool
	}{{
		containerType: instance.LXC,
		valid:         true,
	}, {
		containerType: instance.KVM,
		valid:         true,
	}, {
		containerType: instance.NONE,
		valid:         false,
	}, {
		containerType: instance.ContainerType("other"),
		valid:         false,
	}} {
		manager, err := factory.NewContainerManager(test.containerType, conf)
		if test.valid {
			c.Assert(err, gc.IsNil)
			c.Assert(manager, gc.NotNil)
		} else {
			c.Assert(err, gc.ErrorMatches, `unknown container type: ".*"`)
			c.Assert(manager, gc.IsNil)
		}
	}
}
