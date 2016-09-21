// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"time"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/application"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	"github.com/juju/juju/status"
	"github.com/juju/juju/testcharms"
	"github.com/juju/juju/testing"
)

type ResolvedSuite struct {
	jujutesting.RepoSuite
	testing.CmdBlockHelper
}

func (s *ResolvedSuite) SetUpTest(c *gc.C) {
	s.RepoSuite.SetUpTest(c)
	s.CmdBlockHelper = testing.NewCmdBlockHelper(s.APIState)
	c.Assert(s.CmdBlockHelper, gc.NotNil)
	s.AddCleanup(func(*gc.C) { s.CmdBlockHelper.Close() })
}

var _ = gc.Suite(&ResolvedSuite{})

func runResolved(c *gc.C, args []string) error {
	_, err := testing.RunCommand(c, newResolvedCommand(), args...)
	return err
}

func runDeploy(c *gc.C, args ...string) error {
	_, err := testing.RunCommand(c, application.NewDeployCommand(), args...)
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
		err:  `unit "jeremy-fisher/99" not found \(not found\)`,
	}, {
		args: []string{"dummy/0"},
		err:  `unit "dummy/0" is not in an error state`,
		unit: "dummy/0",
		mode: state.ResolvedNone,
	}, {
		args: []string{"dummy/1", "--no-retry"},
		err:  `unit "dummy/1" is not in an error state`,
		unit: "dummy/1",
		mode: state.ResolvedNone,
	}, {
		args: []string{"dummy/2", "--no-retry"},
		unit: "dummy/2",
		mode: state.ResolvedNoHooks,
	}, {
		args: []string{"dummy/2", "--no-retry"},
		err:  `cannot set resolved mode for unit "dummy/2": already resolved`,
		unit: "dummy/2",
		mode: state.ResolvedNoHooks,
	}, {
		args: []string{"dummy/3"},
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
	ch := testcharms.Repo.CharmArchivePath(s.CharmsPath, "dummy")
	err := runDeploy(c, "-n", "5", ch, "dummy", "--series", "quantal")
	c.Assert(err, jc.ErrorIsNil)

	// lp:1558657
	now := time.Now()
	for _, name := range []string{"dummy/2", "dummy/3", "dummy/4"} {
		u, err := s.State.Unit(name)
		c.Assert(err, jc.ErrorIsNil)
		sInfo := status.StatusInfo{
			Status:  status.Error,
			Message: "lol borken",
			Since:   &now,
		}
		err = u.SetAgentStatus(sInfo)
		c.Assert(err, jc.ErrorIsNil)
	}

	for i, t := range resolvedTests {
		c.Logf("test %d: %v", i, t.args)
		err := runResolved(c, t.args)
		if t.err != "" {
			c.Assert(err, gc.ErrorMatches, t.err)
		} else {
			c.Assert(err, jc.ErrorIsNil)
		}
		if t.unit != "" {
			unit, err := s.State.Unit(t.unit)
			c.Assert(err, jc.ErrorIsNil)
			c.Assert(unit.Resolved(), gc.Equals, t.mode)
		}
	}
}

func (s *ResolvedSuite) TestBlockResolved(c *gc.C) {
	ch := testcharms.Repo.CharmArchivePath(s.CharmsPath, "dummy")
	err := runDeploy(c, "-n", "5", ch, "dummy", "--series", "quantal")
	c.Assert(err, jc.ErrorIsNil)

	// lp:1558657
	now := time.Now()
	for _, name := range []string{"dummy/2", "dummy/3", "dummy/4"} {
		u, err := s.State.Unit(name)
		c.Assert(err, jc.ErrorIsNil)
		sInfo := status.StatusInfo{
			Status:  status.Error,
			Message: "lol borken",
			Since:   &now,
		}
		err = u.SetAgentStatus(sInfo)
		c.Assert(err, jc.ErrorIsNil)
	}

	// Block operation
	s.BlockAllChanges(c, "TestBlockResolved")
	err = runResolved(c, []string{"dummy/2"})
	s.AssertBlocked(c, err, ".*TestBlockResolved.*")
}
