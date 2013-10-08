// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"fmt"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/errors"
	"launchpad.net/juju-core/state"
	jc "launchpad.net/juju-core/testing/checkers"
	"launchpad.net/juju-core/tools"
	"launchpad.net/juju-core/version"
)

type tooler interface {
	AgentTools() (*tools.Tools, error)
	SetAgentVersion(v version.Binary) error
	Life() state.Life
	Refresh() error
	Destroy() error
	EnsureDead() error
}

var _ = gc.Suite(&ToolsSuite{})

type ToolsSuite struct {
	ConnSuite
}

func newTools(vers, url string) *tools.Tools {
	return &tools.Tools{
		Version: version.MustParseBinary(vers),
		URL:     url,
		Size:    10,
		SHA256:  "1234",
	}
}

func testAgentTools(c *gc.C, obj tooler, agent string) {
	// object starts with zero'd tools.
	t, err := obj.AgentTools()
	c.Assert(t, gc.IsNil)
	c.Assert(err, jc.Satisfies, errors.IsNotFoundError)

	err = obj.SetAgentVersion(version.Binary{})
	c.Assert(err, gc.ErrorMatches, fmt.Sprintf("cannot set agent version for %s: empty series or arch", agent))

	v2 := version.MustParseBinary("7.8.9-foo-bar")
	err = obj.SetAgentVersion(v2)
	c.Assert(err, gc.IsNil)
	t3, err := obj.AgentTools()
	c.Assert(err, gc.IsNil)
	c.Assert(t3.Version, gc.DeepEquals, v2)
	err = obj.Refresh()
	c.Assert(err, gc.IsNil)
	t3, err = obj.AgentTools()
	c.Assert(err, gc.IsNil)
	c.Assert(t3.Version, gc.DeepEquals, v2)

	testWhenDying(c, obj, noErr, deadErr, func() error {
		return obj.SetAgentVersion(v2)
	})
}

func (s *ToolsSuite) TestMachineAgentTools(c *gc.C) {
	m, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, gc.IsNil)
	testAgentTools(c, m, "machine 0")
}

func (s *ToolsSuite) TestUnitAgentTools(c *gc.C) {
	charm := s.AddTestingCharm(c, "dummy")
	svc := s.AddTestingService(c, "wordpress", charm)
	unit, err := svc.AddUnit()
	c.Assert(err, gc.IsNil)
	preventUnitDestroyRemove(c, unit)
	testAgentTools(c, unit, `unit "wordpress/0"`)
}
