// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent_test

import (
	"fmt"

	jc "github.com/juju/testing/checkers"
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/utils"
)

var _ = gc.Suite(&unitSuite{})

type unitSuite struct {
	testing.JujuConnSuite
	unit *state.Unit
	st   *api.State
}

func (s *unitSuite) SetUpTest(c *gc.C) {
	var err error
	s.JujuConnSuite.SetUpTest(c)
	svc := s.AddTestingService(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	s.unit, err = svc.AddUnit()
	c.Assert(err, gc.IsNil)
	password, err := utils.RandomPassword()
	c.Assert(err, gc.IsNil)
	err = s.unit.SetPassword(password)

	s.st = s.OpenAPIAs(c, s.unit.Tag(), password)
}

func (s *unitSuite) TestUnitEntity(c *gc.C) {
	m, err := s.st.Agent().Entity("wordpress/1")
	c.Assert(err, gc.ErrorMatches, "permission denied")
	c.Assert(err, jc.Satisfies, params.IsCodeUnauthorized)
	c.Assert(m, gc.IsNil)

	m, err = s.st.Agent().Entity(s.unit.Tag())
	c.Assert(err, gc.IsNil)
	c.Assert(m.Tag(), gc.Equals, s.unit.Tag())
	c.Assert(m.Life(), gc.Equals, params.Alive)
	c.Assert(m.Jobs(), gc.HasLen, 0)

	err = s.unit.EnsureDead()
	c.Assert(err, gc.IsNil)
	err = s.unit.Remove()
	c.Assert(err, gc.IsNil)

	m, err = s.st.Agent().Entity(s.unit.Tag())
	c.Assert(err, gc.ErrorMatches, fmt.Sprintf("unit %q not found", s.unit.Name()))
	c.Assert(err, jc.Satisfies, params.IsCodeNotFound)
	c.Assert(m, gc.IsNil)
}
