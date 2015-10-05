// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resolver_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6-unstable/hooks"

	"github.com/juju/juju/worker/uniter/hook"
	"github.com/juju/juju/worker/uniter/operation"
	"github.com/juju/juju/worker/uniter/resolver"
)

type LockerSuite struct {
	locker *mockCharmDirLocker
}

var _ = gc.Suite(&LockerSuite{})

func (s *LockerSuite) SetUpTest(c *gc.C) {
	s.locker = &mockCharmDirLocker{}
}

func (s *LockerSuite) TestNotAvailable(c *gc.C) {
	resolver.UpdateCharmDir(operation.State{}, s.locker)
	resolver.UpdateCharmDir(operation.State{Started: false}, s.locker)
	resolver.UpdateCharmDir(operation.State{Started: true, Stopped: true}, s.locker)
	resolver.UpdateCharmDir(operation.State{Started: false}, s.locker)
	resolver.UpdateCharmDir(operation.State{Started: true, Stopped: false, Kind: operation.Install}, s.locker)
	resolver.UpdateCharmDir(operation.State{Started: true, Stopped: false, Kind: operation.Upgrade}, s.locker)
	resolver.UpdateCharmDir(operation.State{
		Started: true,
		Stopped: false,
		Kind:    operation.RunHook,
		Hook: &hook.Info{
			Kind: hooks.UpgradeCharm,
		},
	}, s.locker)

	c.Assert(s.locker.Calls(), gc.HasLen, 7)
	for _, call := range s.locker.Calls() {
		c.Assert(call.Args, jc.SameContents, []interface{}{false})
	}
}

func (s *LockerSuite) TestAvailable(c *gc.C) {
	resolver.UpdateCharmDir(operation.State{Started: true, Stopped: false}, s.locker)
	resolver.UpdateCharmDir(operation.State{Started: true, Stopped: false, Kind: operation.Continue}, s.locker)
	resolver.UpdateCharmDir(operation.State{Started: true, Stopped: false, Kind: operation.RunAction}, s.locker)
	resolver.UpdateCharmDir(operation.State{
		Started: true,
		Stopped: false,
		Kind:    operation.RunHook,
		Hook: &hook.Info{
			Kind: hooks.ConfigChanged,
		},
	}, s.locker)

	c.Assert(s.locker.Calls(), gc.HasLen, 4)
	for _, call := range s.locker.Calls() {
		c.Assert(call.Args, jc.SameContents, []interface{}{true})
	}
}
