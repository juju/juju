// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxc_test

import (
	stdtesting "testing"

	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/container/lxc"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/testing"
)

func Test(t *stdtesting.T) { TestingT(t) }

type LxcSuite struct {
	testing.LoggingSuite
}

var _ = Suite(&LxcSuite{})

func (s *LxcSuite) TestNewContainer(c *C) {
	factory := lxc.NewFactory(MockFactory())
	container, err := factory.NewContainer("2/lxc/0")
	c.Assert(err, IsNil)
	c.Assert(container.Id(), Equals, instance.Id("machine-2-lxc-0"))
}
