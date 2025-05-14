// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resolver_test

import (
	"github.com/juju/tc"

	"github.com/juju/juju/internal/charm/hooks"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/worker/uniter/hook"
	"github.com/juju/juju/internal/worker/uniter/operation"
	"github.com/juju/juju/internal/worker/uniter/resolver"
)

type GuardSuite struct {
	guard *mockCharmDirGuard
}

var _ = tc.Suite(&GuardSuite{})

func (s *GuardSuite) SetUpTest(c *tc.C) {
	s.guard = &mockCharmDirGuard{}
}

func (s *GuardSuite) checkCall(c *tc.C, state operation.State, call string) {
	err := resolver.UpdateCharmDir(c.Context(), state, s.guard, loggertesting.WrapCheckLog(c))
	c.Assert(err, tc.ErrorIsNil)
	s.guard.CheckCallNames(c, call)
}

func (s *GuardSuite) TestLockdownEmptyState(c *tc.C) {
	s.checkCall(c, operation.State{}, "Lockdown")
}

func (s *GuardSuite) TestLockdownNotStarted(c *tc.C) {
	s.checkCall(c, operation.State{Started: false}, "Lockdown")
}

func (s *GuardSuite) TestLockdownStartStopInvalid(c *tc.C) {
	s.checkCall(c, operation.State{Started: true, Stopped: true}, "Lockdown")
}

func (s *GuardSuite) TestLockdownInstall(c *tc.C) {
	s.checkCall(c, operation.State{Started: true, Stopped: false, Kind: operation.Install}, "Lockdown")
}

func (s *GuardSuite) TestLockdownUpgrade(c *tc.C) {
	s.checkCall(c, operation.State{Started: true, Stopped: false, Kind: operation.Upgrade}, "Lockdown")
}

func (s *GuardSuite) TestLockdownRunHookUpgradeCharm(c *tc.C) {
	s.checkCall(c, operation.State{
		Started: true,
		Stopped: false,
		Kind:    operation.RunHook,
		Hook: &hook.Info{
			Kind: hooks.UpgradeCharm,
		},
	}, "Lockdown")
}

func (s *GuardSuite) TestUnlockStarted(c *tc.C) {
	s.checkCall(c, operation.State{Started: true, Stopped: false}, "Unlock")
}

func (s *GuardSuite) TestUnlockStartedContinue(c *tc.C) {
	s.checkCall(c, operation.State{Started: true, Stopped: false, Kind: operation.Continue}, "Unlock")
}

func (s *GuardSuite) TestUnlockStartedRunAction(c *tc.C) {
	s.checkCall(c, operation.State{Started: true, Stopped: false, Kind: operation.RunAction}, "Unlock")
}

func (s *GuardSuite) TestUnlockConfigChanged(c *tc.C) {
	s.checkCall(c, operation.State{
		Started: true,
		Stopped: false,
		Kind:    operation.RunHook,
		Hook: &hook.Info{
			Kind: hooks.ConfigChanged,
		},
	}, "Unlock")
}
