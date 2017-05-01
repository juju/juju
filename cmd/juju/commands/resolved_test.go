// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"time"

	"github.com/juju/cmd/cmdtesting"
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
	_, err := cmdtesting.RunCommand(c, newResolvedCommand(), args...)
	return err
}

func runDeploy(c *gc.C, args ...string) error {
	_, err := cmdtesting.RunCommand(c, application.NewDeployCommand(), args...)
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
		args: []string{"multi-series/0"},
		err:  `unit "multi-series/0" is not in an error state`,
		unit: "multi-series/0",
		mode: state.ResolvedNone,
	}, {
		args: []string{"multi-series/1", "--no-retry"},
		err:  `unit "multi-series/1" is not in an error state`,
		unit: "multi-series/1",
		mode: state.ResolvedNone,
	}, {
		args: []string{"multi-series/2", "--no-retry"},
		unit: "multi-series/2",
		mode: state.ResolvedNoHooks,
	}, {
		args: []string{"multi-series/2", "--no-retry"},
		err:  `cannot set resolved mode for unit "multi-series/2": already resolved`,
		unit: "multi-series/2",
		mode: state.ResolvedNoHooks,
	}, {
		args: []string{"multi-series/3"},
		unit: "multi-series/3",
		mode: state.ResolvedRetryHooks,
	}, {
		args: []string{"multi-series/3"},
		err:  `cannot set resolved mode for unit "multi-series/3": already resolved`,
		unit: "multi-series/3",
		mode: state.ResolvedRetryHooks,
	}, {
		args: []string{"multi-series/4", "roflcopter"},
		err:  `unrecognized args: \["roflcopter"\]`,
	},
}

func (s *ResolvedSuite) TestResolved(c *gc.C) {
	ch := testcharms.Repo.CharmArchivePath(s.CharmsPath, "multi-series")
	err := runDeploy(c, "-n", "5", ch, "multi-series")
	c.Assert(err, jc.ErrorIsNil)

	// lp:1558657
	now := time.Now()
	for _, name := range []string{"multi-series/2", "multi-series/3", "multi-series/4"} {
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
	ch := testcharms.Repo.CharmArchivePath(s.CharmsPath, "multi-series")
	err := runDeploy(c, "-n", "5", ch, "multi-series")
	c.Assert(err, jc.ErrorIsNil)

	// lp:1558657
	now := time.Now()
	for _, name := range []string{"multi-series/2", "multi-series/3", "multi-series/4"} {
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
	err = runResolved(c, []string{"multi-series/2"})
	testing.AssertOperationWasBlocked(c, err, ".*TestBlockResolved.*")
}
