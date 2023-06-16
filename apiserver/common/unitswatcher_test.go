// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"fmt"

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
	coretesting "github.com/juju/juju/testing"
)

type unitsWatcherSuite struct {
	coretesting.BaseSuite

	watcherRegistry facade.WatcherRegistry
}

var _ = gc.Suite(&unitsWatcherSuite{})

type fakeUnitsWatcher struct {
	state.UnitsWatcher
	initial []string
	fetchError
}

func (f *fakeUnitsWatcher) WatchUnits() state.StringsWatcher {
	changes := make(chan []string, 1)
	// Simulate initial event.
	changes <- f.initial
	return &fakeStringsWatcher{changes}
}

type fakeStringsWatcher struct {
	changes chan []string
}

func (*fakeStringsWatcher) Stop() error {
	return nil
}

func (*fakeStringsWatcher) Kill() {}

func (*fakeStringsWatcher) Wait() error {
	return nil
}

func (*fakeStringsWatcher) Err() error {
	return nil
}

func (w *fakeStringsWatcher) Changes() <-chan []string {
	return w.changes
}

func (s *unitsWatcherSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	var err error
	s.watcherRegistry, err = registry.NewRegistry(clock.WallClock)
	c.Assert(err, jc.ErrorIsNil)
	s.AddCleanup(func(_ *gc.C) { workertest.DirtyKill(c, s.watcherRegistry) })
}

func (s *unitsWatcherSuite) TestWatchUnits(c *gc.C) {
	st := &fakeState{
		entities: map[names.Tag]entityWithError{
			u("x/0"): &fakeUnitsWatcher{fetchError: "x0 fails"},
			u("x/1"): &fakeUnitsWatcher{initial: []string{"foo", "bar"}},
			u("x/2"): &fakeUnitsWatcher{},
		},
	}
	getCanWatch := func() (common.AuthFunc, error) {
		x0 := u("x/0")
		x1 := u("x/1")
		return func(tag names.Tag) bool {
			return tag == x0 || tag == x1
		}, nil
	}

	w := common.NewUnitsWatcher(st, s.watcherRegistry, getCanWatch)
	entities := params.Entities{
		Entities: []params.Entity{
			{Tag: "unit-x-0"},
			{Tag: "unit-x-1"},
			{Tag: "unit-x-2"},
			{Tag: "unit-x-3"},
		},
	}
	result, err := w.WatchUnits(entities)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, params.StringsWatchResults{
		Results: []params.StringsWatchResult{
			{Error: &params.Error{Message: "x0 fails"}},
			{StringsWatcherId: "1", Changes: []string{"foo", "bar"}, Error: nil},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})
}

func (s *unitsWatcherSuite) TestWatchUnitsError(c *gc.C) {
	getCanWatch := func() (common.AuthFunc, error) {
		return nil, fmt.Errorf("pow")
	}
	w := common.NewUnitsWatcher(
		&fakeState{},
		s.watcherRegistry,
		getCanWatch,
	)
	_, err := w.WatchUnits(params.Entities{Entities: []params.Entity{{Tag: "x0"}}})
	c.Assert(err, gc.ErrorMatches, "pow")
}

func (s *unitsWatcherSuite) TestWatchNoArgsNoError(c *gc.C) {
	getCanWatch := func() (common.AuthFunc, error) {
		return nil, fmt.Errorf("pow")
	}
	w := common.NewUnitsWatcher(
		&fakeState{},
		s.watcherRegistry,
		getCanWatch,
	)
	result, err := w.WatchUnits(params.Entities{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 0)
}
