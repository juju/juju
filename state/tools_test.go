// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"fmt"

	. "launchpad.net/gocheck"

	"launchpad.net/juju-core/agent/tools"
	"launchpad.net/juju-core/errors"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/testing/checkers"
	"launchpad.net/juju-core/version"
)

type tooler interface {
	AgentTools() (*tools.Tools, error)
	SetAgentTools(t *tools.Tools) error
	Life() state.Life
	Refresh() error
	Destroy() error
	EnsureDead() error
}

var _ = Suite(&ToolsSuite{})

type ToolsSuite struct {
	ConnSuite
}

func newTools(vers, url string) *tools.Tools {
	return &tools.Tools{
		Binary: version.MustParseBinary(vers),
		URL:    url,
	}
}

func testAgentTools(c *C, obj tooler, agent string) {
	// object starts with zero'd tools.
	t, err := obj.AgentTools()
	c.Assert(t, IsNil)
	c.Assert(err, checkers.Satisfies, errors.IsNotFoundError)

	err = obj.SetAgentTools(&tools.Tools{})
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

	testWhenDying(c, obj, noErr, deadErr, func() error {
		return obj.SetAgentTools(t2)
	})
}

func (s *ToolsSuite) TestMachineAgentTools(c *C) {
	m, err := s.State.AddMachine("series", state.JobHostUnits)
	c.Assert(err, IsNil)
	testAgentTools(c, m, "machine 0")
}

func (s *ToolsSuite) TestUnitAgentTools(c *C) {
	charm := s.AddTestingCharm(c, "dummy")
	svc, err := s.State.AddService("wordpress", charm)
	c.Assert(err, IsNil)
	unit, err := svc.AddUnit()
	c.Assert(err, IsNil)
	preventUnitDestroyRemove(c, unit)
	testAgentTools(c, unit, `unit "wordpress/0"`)
}
