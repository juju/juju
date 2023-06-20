// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"github.com/juju/clock"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v3/workertest"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/watcher/registry"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing"
)

type modelMachinesWatcherSuite struct {
	testing.BaseSuite

	watcherRegistry facade.WatcherRegistry
}

var _ = gc.Suite(&modelMachinesWatcherSuite{})

type fakeModelMachinesWatcher struct {
	state.ModelMachinesWatcher
	initial []string
}

func (f *fakeModelMachinesWatcher) WatchModelMachines() state.StringsWatcher {
	changes := make(chan []string, 1)
	// Simulate initial event.
	changes <- f.initial
	return &fakeStringsWatcher{changes: changes}
}

func (s *modelMachinesWatcherSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	var err error
	s.watcherRegistry, err = registry.NewRegistry(clock.WallClock)
	c.Assert(err, jc.ErrorIsNil)
	s.AddCleanup(func(c *gc.C) { workertest.DirtyKill(c, s.watcherRegistry) })
}

func (s *modelMachinesWatcherSuite) TestWatchModelMachines(c *gc.C) {
	authorizer := apiservertesting.FakeAuthorizer{
		Tag:        names.NewMachineTag("0"),
		Controller: true,
	}

	e := common.NewModelMachinesWatcher(
		&fakeModelMachinesWatcher{initial: []string{"foo"}},
		s.watcherRegistry,
		authorizer,
	)
	result, err := e.WatchModelMachines()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, params.StringsWatchResult{StringsWatcherId: "1", Changes: []string{"foo"}, Error: nil})
	c.Assert(s.watcherRegistry.Count(), gc.Equals, 1)
}

func (s *modelMachinesWatcherSuite) TestWatchAuthError(c *gc.C) {
	authorizer := apiservertesting.FakeAuthorizer{
		Tag:        names.NewMachineTag("1"),
		Controller: false,
	}
	e := common.NewModelMachinesWatcher(
		&fakeModelMachinesWatcher{},
		s.watcherRegistry,
		authorizer,
	)
	_, err := e.WatchModelMachines()
	c.Assert(err, gc.ErrorMatches, "permission denied")
	c.Assert(s.watcherRegistry.Count(), gc.Equals, 0)
}
