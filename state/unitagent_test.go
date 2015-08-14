// Copyright 2012-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"time"

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

	statusInfo, err := agent.Status()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(statusInfo.Status, gc.Equals, state.StatusAllocating)
	c.Assert(statusInfo.Message, gc.Equals, "")
	c.Assert(statusInfo.Data, gc.HasLen, 0)

	err = agent.SetStatus(state.StatusIdle, "", nil)
	c.Assert(err, jc.ErrorIsNil)
	statusInfo, err = agent.Status()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(statusInfo.Status, gc.Equals, state.StatusIdle)
	c.Assert(statusInfo.Message, gc.Equals, "")
	c.Assert(statusInfo.Data, gc.HasLen, 0)

	err = agent.SetStatus(state.StatusError, "test-hook failed", map[string]interface{}{
		"foo": "bar",
	})
	c.Assert(err, jc.ErrorIsNil)
	// Agent error is reported as unit error.
	statusInfo, err = s.unit.Status()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(statusInfo.Status, gc.Equals, state.StatusError)
	c.Assert(statusInfo.Message, gc.Equals, "test-hook failed")
	c.Assert(statusInfo.Data, gc.DeepEquals, map[string]interface{}{
		"foo": "bar",
	})
	// For agents, error is reported as idle.
	statusInfo, err = agent.Status()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(statusInfo.Status, gc.Equals, state.StatusIdle)
	c.Assert(statusInfo.Message, gc.Equals, "")
	c.Assert(statusInfo.Data, gc.DeepEquals, map[string]interface{}{})
}

func (s *UnitAgentSuite) TestGetSetStatusWhileNotAlive(c *gc.C) {
	agent := s.unit.Agent().(*state.UnitAgent)
	err := s.unit.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	err = agent.SetStatus(state.StatusIdle, "not really", nil)
	c.Assert(err, gc.ErrorMatches, `cannot set status of unit agent "wordpress/0": not found or dead`)
	_, err = agent.Status()
	c.Assert(err, gc.ErrorMatches, `status for key "u#wordpress/0" not found`)

	err = s.unit.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = agent.SetStatus(state.StatusIdle, "not really", nil)
	c.Assert(err, gc.ErrorMatches, `cannot set status of unit agent "wordpress/0": not found or dead`)
	_, err = agent.Status()
	c.Assert(err, gc.ErrorMatches, `status for key "u#wordpress/0" not found`)
}

func (s *UnitAgentSuite) TestGetSetStatusDataStandard(c *gc.C) {
	agent := s.unit.Agent().(*state.UnitAgent)
	err := agent.SetStatus(state.StatusIdle, "", nil)
	c.Assert(err, jc.ErrorIsNil)
	_, err = agent.Status()
	c.Assert(err, jc.ErrorIsNil)

	// Regular status setting with data.
	err = agent.SetStatus(state.StatusError, "test-hook failed", map[string]interface{}{
		"1st-key": "one",
		"2nd-key": 2,
		"3rd-key": true,
	})
	c.Assert(err, jc.ErrorIsNil)

	// Agent error is reported as unit error.
	statusInfo, err := s.unit.Status()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(statusInfo.Status, gc.Equals, state.StatusError)
	c.Assert(statusInfo.Message, gc.Equals, "test-hook failed")
	c.Assert(statusInfo.Data, gc.DeepEquals, map[string]interface{}{
		"1st-key": "one",
		"2nd-key": 2,
		"3rd-key": true,
	})
}

func timeBeforeOrEqual(timeBefore, timeOther time.Time) bool {
	return timeBefore.Before(timeOther) || timeBefore.Equal(timeOther)
}

func (s *UnitAgentSuite) TestSetAgentStatusSince(c *gc.C) {
	agent := s.unit.Agent().(*state.UnitAgent)

	now := state.NowToTheSecond()
	err := agent.SetStatus(state.StatusIdle, "", nil)
	c.Assert(err, jc.ErrorIsNil)
	statusInfo, err := agent.Status()
	c.Assert(err, jc.ErrorIsNil)
	firstTime := statusInfo.Since
	c.Assert(firstTime, gc.NotNil)
	c.Assert(timeBeforeOrEqual(now, *firstTime), jc.IsTrue)

	// Setting the same status a second time also updates the timestamp.
	err = agent.SetStatus(state.StatusIdle, "", nil)
	c.Assert(err, jc.ErrorIsNil)
	statusInfo, err = agent.Status()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(timeBeforeOrEqual(*firstTime, *statusInfo.Since), jc.IsTrue)
}

func (s *UnitAgentSuite) TestSetUnitAgentStatusHistory(c *gc.C) {
	err := state.EraseUnitHistory(s.unit)
	c.Assert(err, jc.ErrorIsNil)

	agent := s.unit.Agent().(*state.UnitAgent)
	globalKey := state.UnitAgentGlobalKey(agent)
	err = agent.SetStatus(state.StatusIdle, "to push something to history", nil)
	c.Assert(err, jc.ErrorIsNil)
	statusInfo, err := agent.Status()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(statusInfo.Status, gc.Equals, state.StatusIdle)

	h, err := state.StatusHistory(10, globalKey, s.State)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(h, gc.HasLen, 1)
	c.Assert(h[0].Status, gc.Equals, state.StatusAllocating)
	c.Assert(h[0].Message, gc.Equals, "")

	err = agent.SetStatus(state.StatusExecuting, "executing first thing", nil)
	c.Assert(err, jc.ErrorIsNil)
	statusInfo, err = agent.Status()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(statusInfo.Status, gc.Equals, state.StatusExecuting)

	err = agent.SetStatus(state.StatusExecuting, "wow executing again", nil)
	c.Assert(err, jc.ErrorIsNil)
	statusInfo, err = agent.Status()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(statusInfo.Status, gc.Equals, state.StatusExecuting)

	h, err = state.StatusHistory(10, globalKey, s.State)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(h, gc.HasLen, 3)
	var message string
	for i := 0; i < 3; i++ {
		c.Logf("checking status %q", h[i].Status)
		switch h[i].Status {
		case state.StatusAllocating:
			message = ""
		case state.StatusExecuting:
			message = "executing first thing"
		case state.StatusIdle:
			message = "to push something to history"
		}
		c.Assert(h[i].Message, gc.Equals, message)
	}
}

func (s *UnitAgentSuite) TestGetUnitAgentStatusHistory(c *gc.C) {
	err := state.EraseUnitHistory(s.unit)
	c.Assert(err, jc.ErrorIsNil)
	agent := s.unit.Agent().(*state.UnitAgent)
	globalKey := state.UnitAgentGlobalKey(agent)
	history := func(i int) ([]state.StatusInfo, error) {
		return agent.StatusHistory(i)
	}
	testGetUnitStatusHistory(c, history, s.State, globalKey)
}

func (s *UnitAgentSuite) TestGetSetStatusDataMongo(c *gc.C) {
	agent := s.unit.Agent().(*state.UnitAgent)
	err := agent.SetStatus(state.StatusIdle, "", nil)
	c.Assert(err, jc.ErrorIsNil)
	_, err = agent.Status()
	c.Assert(err, jc.ErrorIsNil)

	// Status setting with MongoDB special values.
	err = agent.SetStatus(state.StatusError, "mongo", map[string]interface{}{
		`{name: "Joe"}`: "$where",
		"eval":          `eval(function(foo) { return foo; }, "bar")`,
		"mapReduce":     "mapReduce",
		"group":         "group",
	})
	c.Assert(err, jc.ErrorIsNil)

	// Agent error is reported as unit error.
	statusInfo, err := s.unit.Status()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(statusInfo.Status, gc.Equals, state.StatusError)
	c.Assert(statusInfo.Message, gc.Equals, "mongo")
	c.Assert(statusInfo.Data, gc.DeepEquals, map[string]interface{}{
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
	_, err = agent.Status()
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

	// Agent error is reported as unit error.
	statusInfo, err := s.unit.Status()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(statusInfo.Status, gc.Equals, state.StatusError)
	c.Assert(statusInfo.Message, gc.Equals, "test-hook failed")
	c.Assert(statusInfo.Data, gc.DeepEquals, map[string]interface{}{
		"1st-key": "one",
		"2nd-key": 2,
		"3rd-key": true,
	})

	// Set status data to nil, so an empty map will be returned.
	err = agent.SetStatus(state.StatusIdle, "", nil)
	c.Assert(err, jc.ErrorIsNil)

	statusInfo, err = agent.Status()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(statusInfo.Status, gc.Equals, state.StatusIdle)
	c.Assert(statusInfo.Message, gc.Equals, "")
	c.Assert(statusInfo.Data, gc.HasLen, 0)
}
