// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent_test

import (
	"fmt"

	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
)

var _ = gc.Suite(&unitSuite{})

type unitSuite struct {
	testing.JujuConnSuite
	unit *state.Unit
	st   api.Connection
}

func (s *unitSuite) SetUpTest(c *gc.C) {
	var err error
	s.JujuConnSuite.SetUpTest(c)
	svc := s.AddTestingService(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	s.unit, err = svc.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	password, err := utils.RandomPassword()
	c.Assert(err, jc.ErrorIsNil)
	err = s.unit.SetPassword(password)
	c.Assert(err, jc.ErrorIsNil)

	s.st = s.OpenAPIAs(c, s.unit.Tag(), password)
}

func (s *unitSuite) TestUnitEntity(c *gc.C) {
	tag := names.NewUnitTag("wordpress/1")
	m, err := s.st.Agent().Entity(tag)
	c.Assert(err, gc.ErrorMatches, "permission denied")
	c.Assert(err, jc.Satisfies, params.IsCodeUnauthorized)
	c.Assert(m, gc.IsNil)

	m, err = s.st.Agent().Entity(s.unit.Tag())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(m.Tag(), gc.Equals, s.unit.Tag().String())
	c.Assert(m.Life(), gc.Equals, params.Alive)
	c.Assert(m.Jobs(), gc.HasLen, 0)

	err = s.unit.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = s.unit.Remove()
	c.Assert(err, jc.ErrorIsNil)

	m, err = s.st.Agent().Entity(s.unit.Tag())
	c.Assert(err, gc.ErrorMatches, fmt.Sprintf("unit %q not found", s.unit.Name()))
	c.Assert(err, jc.Satisfies, params.IsCodeNotFound)
	c.Assert(m, gc.IsNil)
}
