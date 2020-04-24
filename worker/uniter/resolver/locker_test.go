// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resolver_test

import (
	"github.com/juju/charm/v7/hooks"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/worker/fortress"
	"github.com/juju/juju/worker/uniter/hook"
	"github.com/juju/juju/worker/uniter/operation"
	"github.com/juju/juju/worker/uniter/resolver"
)

type GuardSuite struct {
	guard *mockCharmDirGuard
}

var _ = gc.Suite(&GuardSuite{})

func (s *GuardSuite) SetUpTest(c *gc.C) {
	s.guard = &mockCharmDirGuard{}
}

func (s *GuardSuite) checkCall(c *gc.C, state operation.State, call string) {
	err := resolver.UpdateCharmDir(state, s.guard, nil)
	c.Assert(err, jc.ErrorIsNil)
	s.guard.CheckCallNames(c, call)
}

func (s *GuardSuite) TestLockdownEmptyState(c *gc.C) {
	s.checkCall(c, operation.State{}, "Lockdown")
}

func (s *GuardSuite) TestLockdownNotStarted(c *gc.C) {
	s.checkCall(c, operation.State{Started: false}, "Lockdown")
}

func (s *GuardSuite) TestLockdownStartStopInvalid(c *gc.C) {
	s.checkCall(c, operation.State{Started: true, Stopped: true}, "Lockdown")
}

func (s *GuardSuite) TestLockdownInstall(c *gc.C) {
	s.checkCall(c, operation.State{Started: true, Stopped: false, Kind: operation.Install}, "Lockdown")
}

func (s *GuardSuite) TestLockdownUpgrade(c *gc.C) {
	s.checkCall(c, operation.State{Started: true, Stopped: false, Kind: operation.Upgrade}, "Lockdown")
}

func (s *GuardSuite) TestLockdownRunHookUpgradeCharm(c *gc.C) {
	s.checkCall(c, operation.State{
		Started: true,
		Stopped: false,
		Kind:    operation.RunHook,
		Hook: &hook.Info{
			Kind: hooks.UpgradeCharm,
		},
	}, "Lockdown")
}

func (s *GuardSuite) TestUnlockStarted(c *gc.C) {
	s.checkCall(c, operation.State{Started: true, Stopped: false}, "Unlock")
}

func (s *GuardSuite) TestUnlockStartedContinue(c *gc.C) {
	s.checkCall(c, operation.State{Started: true, Stopped: false, Kind: operation.Continue}, "Unlock")
}

func (s *GuardSuite) TestUnlockStartedRunAction(c *gc.C) {
	s.checkCall(c, operation.State{Started: true, Stopped: false, Kind: operation.RunAction}, "Unlock")
}

func (s *GuardSuite) TestUnlockConfigChanged(c *gc.C) {
	s.checkCall(c, operation.State{
		Started: true,
		Stopped: false,
		Kind:    operation.RunHook,
		Hook: &hook.Info{
			Kind: hooks.ConfigChanged,
		},
	}, "Unlock")
}

func (s *GuardSuite) TestLockdownAbortArg(c *gc.C) {
	abort := make(fortress.Abort)
	err := resolver.UpdateCharmDir(operation.State{}, s.guard, abort)
	c.Assert(err, jc.ErrorIsNil)
	s.guard.CheckCalls(c, []testing.StubCall{{FuncName: "Lockdown", Args: []interface{}{abort}}})
}
