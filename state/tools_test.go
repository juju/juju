package state_test

import (
	"fmt"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/version"
)

type tooler interface {
	AgentTools() (*state.Tools, error)
	SetAgentTools(t *state.Tools) error
	Kill() error
	Die() error
	Life() state.Life
	Refresh() error
}

var _ = Suite(&ToolsSuite{})

type ToolsSuite struct {
	ConnSuite
}

func newTools(vers, url string) *state.Tools {
	return &state.Tools{
		Binary: version.MustParseBinary(vers),
		URL:    url,
	}
}

func testAgentTools(c *C, obj tooler, agent string) {
	// object starts with zero'd tools.
	t, err := obj.AgentTools()
	c.Assert(err, IsNil)
	c.Assert(t, DeepEquals, &state.Tools{})

	err = obj.SetAgentTools(&state.Tools{})
	c.Assert(err, ErrorMatches, fmt.Sprintf("cannot set agent tools for %s: empty series or arch", agent))
	t2 := newTools("7.8.9-foo-bar", "http://arble.tgz")
	err = obj.SetAgentTools(t2)
	c.Assert(err, IsNil)
	t3, err := obj.AgentTools()
	c.Assert(err, IsNil)
	c.Assert(t3, DeepEquals, t2)
	err = obj.Refresh()
	c.Assert(err, IsNil)
	t3, err = obj.AgentTools()
	c.Assert(err, IsNil)
	c.Assert(t3, DeepEquals, t2)

	testWhenDying(c, obj, noErr, notAliveErr, func() error {
		return obj.SetAgentTools(t2)
	})
}

func (s *ToolsSuite) TestMachineAgentTools(c *C) {
	m, err := s.State.AddMachine()
	c.Assert(err, IsNil)
	testAgentTools(c, m, "machine 0")
}

func (s *ToolsSuite) TestUnitAgentTools(c *C) {
	charm := s.AddTestingCharm(c, "dummy")
	svc, err := s.State.AddService("wordpress", charm)
	c.Assert(err, IsNil)
	unit, err := svc.AddUnit()
	c.Assert(err, IsNil)
	testAgentTools(c, unit, `unit "wordpress/0"`)
}
