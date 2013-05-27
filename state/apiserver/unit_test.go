// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/state/api"
)

func (s *suite) TestUnitRefresh(c *C) {
	s.setUpScenario(c)
	st := s.openAs(c, "unit-wordpress-0")
	defer st.Close()

	u, err := st.Unit("wordpress/0")
	c.Assert(err, IsNil)

	deployer, ok := u.DeployerTag()
	c.Assert(ok, Equals, true)
	c.Assert(deployer, Equals, "machine-1")

	stu, err := s.State.Unit("wordpress/0")
	c.Assert(err, IsNil)
	err = stu.UnassignFromMachine()
	c.Assert(err, IsNil)

	deployer, ok = u.DeployerTag()
	c.Assert(ok, Equals, true)
	c.Assert(deployer, Equals, "machine-1")

	err = u.Refresh()
	c.Assert(err, IsNil)

	deployer, ok = u.DeployerTag()
	c.Assert(ok, Equals, false)
	c.Assert(deployer, Equals, "")
}

func (s *suite) TestUnitTag(c *C) {
	c.Assert(api.UnitTag("wordpress/2"), Equals, "unit-wordpress-2")

	s.setUpScenario(c)
	st := s.openAs(c, "unit-wordpress-0")
	defer st.Close()
	u, err := st.Unit("wordpress/0")
	c.Assert(err, IsNil)
	c.Assert(u.Tag(), Equals, "unit-wordpress-0")
}
