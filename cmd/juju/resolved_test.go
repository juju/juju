// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/cmd/envcmd"
	jujutesting "launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/testing"
)

type ResolvedSuite struct {
	jujutesting.RepoSuite
}

var _ = gc.Suite(&ResolvedSuite{})

func runResolved(c *gc.C, args []string) error {
	_, err := testing.RunCommand(c, envcmd.Wrap(&ResolvedCommand{}), args)
	return err
}

var resolvedTests = []struct {
	args []string
	err  string
	unit string
	mode state.ResolvedMode
}{
	{
		err: `no unit specified`,
	}, {
		args: []string{"jeremy-fisher"},
		err:  `invalid unit name "jeremy-fisher"`,
	}, {
		args: []string{"jeremy-fisher/99"},
		err:  `unit "jeremy-fisher/99" not found`,
	}, {
		args: []string{"dummy/0"},
		err:  `unit "dummy/0" is not in an error state`,
		unit: "dummy/0",
		mode: state.ResolvedNone,
	}, {
		args: []string{"dummy/1", "--retry"},
		err:  `unit "dummy/1" is not in an error state`,
		unit: "dummy/1",
		mode: state.ResolvedNone,
	}, {
		args: []string{"dummy/2"},
		unit: "dummy/2",
		mode: state.ResolvedNoHooks,
	}, {
		args: []string{"dummy/2", "--retry"},
		err:  `cannot set resolved mode for unit "dummy/2": already resolved`,
		unit: "dummy/2",
		mode: state.ResolvedNoHooks,
	}, {
		args: []string{"dummy/3", "--retry"},
		unit: "dummy/3",
		mode: state.ResolvedRetryHooks,
	}, {
		args: []string{"dummy/3"},
		err:  `cannot set resolved mode for unit "dummy/3": already resolved`,
		unit: "dummy/3",
		mode: state.ResolvedRetryHooks,
	}, {
		args: []string{"dummy/4", "roflcopter"},
		err:  `unrecognized args: \["roflcopter"\]`,
	},
}

func (s *ResolvedSuite) TestResolved(c *gc.C) {
	testing.Charms.BundlePath(s.SeriesPath, "dummy")
	err := runDeploy(c, "-n", "5", "local:dummy", "dummy")
	c.Assert(err, gc.IsNil)

	for _, name := range []string{"dummy/2", "dummy/3", "dummy/4"} {
		u, err := s.State.Unit(name)
		c.Assert(err, gc.IsNil)
		err = u.SetStatus(params.StatusError, "lol borken", nil)
		c.Assert(err, gc.IsNil)
	}

	for i, t := range resolvedTests {
		c.Logf("test %d: %v", i, t.args)
		err := runResolved(c, t.args)
		if t.err != "" {
			c.Assert(err, gc.ErrorMatches, t.err)
		} else {
			c.Assert(err, gc.IsNil)
		}
		if t.unit != "" {
			unit, err := s.State.Unit(t.unit)
			c.Assert(err, gc.IsNil)
			c.Assert(unit.Resolved(), gc.Equals, t.mode)
		}
	}
}
