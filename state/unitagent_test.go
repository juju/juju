// Copyright 2012-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/state"
)

type UnitAgentSuite struct {
	ConnSuite
	charm   *state.Charm
	service *state.Service
	unit    *state.Unit
}

var _ = gc.Suite(&UnitAgentSuite{})

func (s *UnitAgentSuite) SetUpTest(c *gc.C) {
	s.ConnSuite.SetUpTest(c)
	s.charm = s.AddTestingCharm(c, "wordpress")
	var err error
	s.service = s.AddTestingService(c, "wordpress", s.charm)
	c.Assert(err, jc.ErrorIsNil)

	s.unit, err = s.service.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.unit.Series(), gc.Equals, "quantal")
}

func (s *UnitAgentSuite) TestGetSetStatusWhileAlive(c *gc.C) {
	agent := s.unit.Agent().(*state.UnitAgent)
	err := agent.SetStatus(state.StatusError, "", nil)
	c.Assert(err, gc.ErrorMatches, `cannot set status "error" without info`)
	err = agent.SetStatus(state.Status("vliegkat"), "orville", nil)
	c.Assert(err, gc.ErrorMatches, `cannot set invalid status "vliegkat"`)

	status, info, data, err := agent.Status()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(status, gc.Equals, state.StatusAllocating)
	c.Assert(info, gc.Equals, "")
	c.Assert(data, gc.HasLen, 0)

	err = agent.SetStatus(state.StatusIdle, "", nil)
	c.Assert(err, jc.ErrorIsNil)
	status, info, data, err = agent.Status()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(status, gc.Equals, state.StatusIdle)
	c.Assert(info, gc.Equals, "")
	c.Assert(data, gc.HasLen, 0)

	err = agent.SetStatus(state.StatusError, "test-hook failed", map[string]interface{}{
		"foo": "bar",
	})
	c.Assert(err, jc.ErrorIsNil)
	status, info, data, err = agent.Status()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(status, gc.Equals, state.StatusError)
	c.Assert(info, gc.Equals, "test-hook failed")
	c.Assert(data, gc.DeepEquals, map[string]interface{}{
		"foo": "bar",
	})
}

func (s *UnitAgentSuite) TestGetSetStatusWhileNotAlive(c *gc.C) {
	agent := s.unit.Agent().(*state.UnitAgent)
	err := s.unit.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	err = agent.SetStatus(state.StatusIdle, "not really", nil)
	c.Assert(err, gc.ErrorMatches, `cannot set status of unit agent "wordpress/0": not found or dead`)
	_, _, _, err = agent.Status()
	c.Assert(err, gc.ErrorMatches, "status not found")

	err = s.unit.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = agent.SetStatus(state.StatusIdle, "not really", nil)
	c.Assert(err, gc.ErrorMatches, `cannot set status of unit agent "wordpress/0": not found or dead`)
	_, _, _, err = agent.Status()
	c.Assert(err, gc.ErrorMatches, "status not found")
}

func (s *UnitAgentSuite) TestGetSetStatusDataStandard(c *gc.C) {
	agent := s.unit.Agent().(*state.UnitAgent)
	err := agent.SetStatus(state.StatusIdle, "", nil)
	c.Assert(err, jc.ErrorIsNil)
	_, _, _, err = agent.Status()
	c.Assert(err, jc.ErrorIsNil)

	// Regular status setting with data.
	err = agent.SetStatus(state.StatusError, "test-hook failed", map[string]interface{}{
		"1st-key": "one",
		"2nd-key": 2,
		"3rd-key": true,
	})
	c.Assert(err, jc.ErrorIsNil)

	status, info, data, err := agent.Status()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(status, gc.Equals, state.StatusError)
	c.Assert(info, gc.Equals, "test-hook failed")
	c.Assert(data, gc.DeepEquals, map[string]interface{}{
		"1st-key": "one",
		"2nd-key": 2,
		"3rd-key": true,
	})
}

func (s *UnitAgentSuite) TestGetSetStatusDataMongo(c *gc.C) {
	agent := s.unit.Agent().(*state.UnitAgent)
	err := agent.SetStatus(state.StatusIdle, "", nil)
	c.Assert(err, jc.ErrorIsNil)
	_, _, _, err = agent.Status()
	c.Assert(err, jc.ErrorIsNil)

	// Status setting with MongoDB special values.
	err = agent.SetStatus(state.StatusError, "mongo", map[string]interface{}{
		`{name: "Joe"}`: "$where",
		"eval":          `eval(function(foo) { return foo; }, "bar")`,
		"mapReduce":     "mapReduce",
		"group":         "group",
	})
	c.Assert(err, jc.ErrorIsNil)

	status, info, data, err := agent.Status()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(status, gc.Equals, state.StatusError)
	c.Assert(info, gc.Equals, "mongo")
	c.Assert(data, gc.DeepEquals, map[string]interface{}{
		`{name: "Joe"}`: "$where",
		"eval":          `eval(function(foo) { return foo; }, "bar")`,
		"mapReduce":     "mapReduce",
		"group":         "group",
	})
}

func (s *UnitAgentSuite) TestGetSetStatusDataChange(c *gc.C) {
	agent := s.unit.Agent().(*state.UnitAgent)
	err := agent.SetStatus(state.StatusIdle, "", nil)
	c.Assert(err, jc.ErrorIsNil)
	_, _, _, err = agent.Status()
	c.Assert(err, jc.ErrorIsNil)

	// Status setting and changing data afterwards.
	data := map[string]interface{}{
		"1st-key": "one",
		"2nd-key": 2,
		"3rd-key": true,
	}
	err = agent.SetStatus(state.StatusError, "test-hook failed", data)
	c.Assert(err, jc.ErrorIsNil)
	data["4th-key"] = 4.0

	status, info, data, err := agent.Status()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(status, gc.Equals, state.StatusError)
	c.Assert(info, gc.Equals, "test-hook failed")
	c.Assert(data, gc.DeepEquals, map[string]interface{}{
		"1st-key": "one",
		"2nd-key": 2,
		"3rd-key": true,
	})

	// Set status data to nil, so an empty map will be returned.
	err = agent.SetStatus(state.StatusIdle, "", nil)
	c.Assert(err, jc.ErrorIsNil)

	status, info, data, err = agent.Status()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(status, gc.Equals, state.StatusIdle)
	c.Assert(info, gc.Equals, "")
	c.Assert(data, gc.HasLen, 0)
}
