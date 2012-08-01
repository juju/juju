package state_test

import (
	"fmt"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/version"
)

type versioner interface {
	AgentTools() (*state.Tools, error)
	SetAgentTools(t *state.Tools) error
	ProposedAgentTools() (*state.Tools, error)
	ProposeAgentTools(t *state.Tools) error
}

var _ = Suite(&ToolsSuite{})

type ToolsSuite struct {
	ConnSuite
}

func testAgentTools(c *C, obj versioner, agent string) {
	// object starts with zero'd tools.
	t, err := obj.AgentTools()
	c.Assert(err, IsNil)
	c.Assert(t, DeepEquals, &state.Tools{})

	t, err = obj.ProposedAgentTools()
	c.Assert(err, IsNil)
	c.Assert(t, DeepEquals, &state.Tools{})

	err = obj.ProposeAgentTools(&state.Tools{})
	c.Assert(err, ErrorMatches, fmt.Sprintf("cannot set proposed tools for %s agent: empty series or arch", agent))
	// check that we can set the version
	t0 := &state.Tools{
		Binary: version.MustParseBinary("5.6.7-precise-amd64"),
		URL:    "http://foo/bar.tgz",
	}
	err = obj.ProposeAgentTools(t0)
	c.Assert(err, IsNil)
	t1, err := obj.ProposedAgentTools()
	c.Assert(err, IsNil)
	c.Assert(t1, DeepEquals, t0)

	err = obj.SetAgentTools(&state.Tools{})
	c.Assert(err, ErrorMatches, fmt.Sprintf("cannot set current tools for %s agent: empty series or arch", agent))
	t2 := &state.Tools{
		Binary: version.MustParseBinary("7.8.9-foo-bar"),
		URL:    "http://arble.tgz",
	}
	err = obj.SetAgentTools(t2)
	c.Assert(err, IsNil)
	t3, err := obj.AgentTools()
	c.Assert(err, IsNil)
	c.Assert(t3, DeepEquals, t2)

	// check there's no cross-talk
	t4, err := obj.ProposedAgentTools()
	c.Assert(err, IsNil)
	c.Assert(t4, DeepEquals, t0)
}

func (s *ToolsSuite) TestMachineAgentTools(c *C) {
	m, err := s.State.AddMachine()
	c.Assert(err, IsNil)
	testAgentTools(c, m, "machine")
}

func (s *ToolsSuite) TestUnitAgentTools(c *C) {
	charm := s.AddTestingCharm(c, "dummy")
	svc, err := s.State.AddService("wordpress", charm)
	c.Assert(err, IsNil)
	unit, err := svc.AddUnit()
	c.Assert(err, IsNil)
	testAgentTools(c, unit, "unit")
}
