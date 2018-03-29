// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"time"

	"github.com/juju/cmd/cmdtesting"
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testcharms"
	coretesting "github.com/juju/juju/testing"
)

type ResolvedCommandSuite struct {
	testing.IsolationSuite
	mockAPI *mockResolvedAPI
}

func (s *ResolvedCommandSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.mockAPI = &mockResolvedAPI{Stub: &testing.Stub{}, version: 6}
	s.mockAPI.resolvedCommandFunc = func(unit string, retry bool) error {
		return s.mockAPI.NextErr()
	}
	s.mockAPI.resolvedApplicationUnitsFunc = func(units []string, retry, all bool) error {
		return s.mockAPI.NextErr()
	}
}

var _ = gc.Suite(&ResolvedCommandSuite{})

func (s *ResolvedCommandSuite) runResolved(c *gc.C, args ...string) error {
	_, err := cmdtesting.RunCommand(c, NewResolvedCommandForTest(s.mockAPI), args...)
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
	}, {
		args: []string{"multi-series/5", "--all"},
		err:  "specify unit or --all option, not both",
		unit: "multi-series/5",
	},
}

func (s *ResolvedCommandSuite) TestResolved(c *gc.C) {

	// lp:1558657
	now := time.Now()
	for _, name := range []string{"multi-series/2", "multi-series/3", "multi-series/4", "multi-series/5"} {
		u, err := s.State.Unit(name)
		c.Assert(err, jc.ErrorIsNil)
		sInfo := status.StatusInfo{
			Status:  status.Error,
			Message: "lol broken",
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

func (s *ResolvedCommandSuite) TestBlockResolved(c *gc.C) {
	ch := testcharms.Repo.CharmArchivePath(s.CharmsPath, "multi-series")
	err := runDeploy(c, "-n", "6", ch, "multi-series")
	c.Assert(err, jc.ErrorIsNil)

	// lp:1558657
	now := time.Now()
	for _, name := range []string{"multi-series/2", "multi-series/3", "multi-series/4", "multi-series/5"} {
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

type mockResolvedAPI struct {
	*testing.Stub
	version int

	resolvedCommandFunc          func(string, bool) error
	resolvedApplicationUnitsFunc func([]string, bool, bool) error
}

func (s mockResolvedAPI) Close() error {
	s.MethodCall(s, "Close")
	return s.NextErr()
}

func (s mockResolvedAPI) BestAPIVersion() int {
	return s.version
}

// These methods are on API V6.
func (s mockResolvedAPI) Resolved(unit string, retry bool) error {
	s.MethodCall(s, "Resolved", unit, retry)
	return s.resolvedCommandFunc(unit, retry)
}

// This method is supported in API < V6
func (s mockResolvedAPI) ResolveApplicationUnits(units []string, retry bool, all bool) error {
	s.MethodCall(s, "ResolveApplicationUnits", units, retry, all)
	return s.resolvedApplicationUnitsFunc(units, retry, all)
}
