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
	statetesting "github.com/juju/juju/state/testing"
	coretesting "github.com/juju/juju/testing"
)

type agentEntityWatcherSuite struct {
	coretesting.BaseSuite

	watcherRegistry facade.WatcherRegistry
}

var _ = gc.Suite(&agentEntityWatcherSuite{})

type fakeAgentEntityWatcher struct {
	state.Entity
	fetchError
}

func (a *fakeAgentEntityWatcher) Watch() state.NotifyWatcher {
	return apiservertesting.NewFakeNotifyWatcher()
}

func (s *agentEntityWatcherSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	var err error
	s.watcherRegistry, err = registry.NewRegistry(clock.WallClock)
	c.Assert(err, jc.ErrorIsNil)
	s.AddCleanup(func(_ *gc.C) { workertest.DirtyKill(c, s.watcherRegistry) })
}

func (s *agentEntityWatcherSuite) TestWatch(c *gc.C) {
	st := &fakeState{
		entities: map[names.Tag]entityWithError{
			u("x/0"): &fakeAgentEntityWatcher{fetchError: "x0 fails"},
			u("x/1"): &fakeAgentEntityWatcher{},
			u("x/2"): &fakeAgentEntityWatcher{},
		},
	}
	getCanWatch := func() (common.AuthFunc, error) {
		x0 := u("x/0")
		x1 := u("x/1")
		return func(tag names.Tag) bool {
			return tag == x0 || tag == x1
		}, nil
	}

	a := common.NewAgentEntityWatcher(st, s.watcherRegistry, getCanWatch)
	entities := params.Entities{
		Entities: []params.Entity{
			{Tag: "unit-x-0"},
			{Tag: "unit-x-1"},
			{Tag: "unit-x-2"},
			{Tag: "unit-x-3"},
		},
	}
	result, err := a.Watch(entities)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.NotifyWatchResults{
		Results: []params.NotifyWatchResult{
			{Error: &params.Error{Message: "x0 fails"}},
			{NotifyWatcherId: "1", Error: nil},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})
}

func (s *agentEntityWatcherSuite) TestWatchError(c *gc.C) {
	getCanWatch := func() (common.AuthFunc, error) {
		return nil, fmt.Errorf("pow")
	}

	a := common.NewAgentEntityWatcher(
		&fakeState{},
		s.watcherRegistry,
		getCanWatch,
	)
	_, err := a.Watch(params.Entities{Entities: []params.Entity{{Tag: "x0"}}})
	c.Assert(err, gc.ErrorMatches, "pow")
}

func (s *agentEntityWatcherSuite) TestWatchNoArgsNoError(c *gc.C) {
	getCanWatch := func() (common.AuthFunc, error) {
		return nil, fmt.Errorf("pow")
	}
	a := common.NewAgentEntityWatcher(
		&fakeState{},
		s.watcherRegistry,
		getCanWatch,
	)
	result, err := a.Watch(params.Entities{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 0)
}

type multiNotifyWatcherSuite struct{}

var _ = gc.Suite(&multiNotifyWatcherSuite{})

func (*multiNotifyWatcherSuite) TestMultiNotifyWatcher(c *gc.C) {
	w0 := apiservertesting.NewFakeNotifyWatcher()
	w1 := apiservertesting.NewFakeNotifyWatcher()

	mw := common.NewMultiNotifyWatcher(w0, w1)
	defer workertest.CleanKill(c, mw)

	wc := statetesting.NewNotifyWatcherC(c, mw)
	wc.AssertOneChange()

	w0.C <- struct{}{}
	wc.AssertOneChange()
	w1.C <- struct{}{}
	wc.AssertOneChange()

	w0.C <- struct{}{}
	w1.C <- struct{}{}
	wc.AssertOneChange()
}

func (*multiNotifyWatcherSuite) TestMultiNotifyWatcherStop(c *gc.C) {
	w0 := apiservertesting.NewFakeNotifyWatcher()
	w1 := apiservertesting.NewFakeNotifyWatcher()

	mw := common.NewMultiNotifyWatcher(w0, w1)
	wc := statetesting.NewNotifyWatcherC(c, mw)
	wc.AssertOneChange()
	statetesting.AssertCanStopWhenSending(c, mw)
	wc.AssertClosed()
}
