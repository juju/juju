// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"fmt"

	"launchpad.net/tomb"

	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
)

type agentEntityWatcherSuite struct{}

var _ = gc.Suite(&agentEntityWatcherSuite{})

type fakeAgentEntityWatcher struct {
	state.Entity
	fetchError
}

func (a *fakeAgentEntityWatcher) Watch() state.NotifyWatcher {
	changes := make(chan struct{}, 1)
	// Simulate initial event.
	changes <- struct{}{}
	return &fakeNotifyWatcher{changes: changes}
}

type fakeNotifyWatcher struct {
	tomb    tomb.Tomb
	changes chan struct{}
}

func (w *fakeNotifyWatcher) Stop() error {
	w.Kill()
	return w.Wait()
}

func (w *fakeNotifyWatcher) Kill() {
	w.tomb.Kill(nil)
	w.tomb.Done()
}

func (w *fakeNotifyWatcher) Wait() error {
	return w.tomb.Wait()
}

func (w *fakeNotifyWatcher) Err() error {
	return w.tomb.Err()
}

func (w *fakeNotifyWatcher) Changes() <-chan struct{} {
	return w.changes
}

func (*agentEntityWatcherSuite) TestWatch(c *gc.C) {
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
	resources := common.NewResources()
	a := common.NewAgentEntityWatcher(st, resources, getCanWatch)
	entities := params.Entities{[]params.Entity{
		{"unit-x-0"}, {"unit-x-1"}, {"unit-x-2"}, {"unit-x-3"},
	}}
	result, err := a.Watch(entities)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.NotifyWatchResults{
		Results: []params.NotifyWatchResult{
			{Error: &params.Error{Message: "x0 fails"}},
			{"1", nil},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})
}

func (*agentEntityWatcherSuite) TestWatchError(c *gc.C) {
	getCanWatch := func() (common.AuthFunc, error) {
		return nil, fmt.Errorf("pow")
	}
	resources := common.NewResources()
	a := common.NewAgentEntityWatcher(
		&fakeState{},
		resources,
		getCanWatch,
	)
	_, err := a.Watch(params.Entities{[]params.Entity{{"x0"}}})
	c.Assert(err, gc.ErrorMatches, "pow")
}

func (*agentEntityWatcherSuite) TestWatchNoArgsNoError(c *gc.C) {
	getCanWatch := func() (common.AuthFunc, error) {
		return nil, fmt.Errorf("pow")
	}
	resources := common.NewResources()
	a := common.NewAgentEntityWatcher(
		&fakeState{},
		resources,
		getCanWatch,
	)
	result, err := a.Watch(params.Entities{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 0)
}

type multiNotifyWatcherSuite struct{}

var _ = gc.Suite(&multiNotifyWatcherSuite{})

func (*multiNotifyWatcherSuite) TestMultiNotifyWatcher(c *gc.C) {
	w0 := &fakeNotifyWatcher{changes: make(chan struct{}, 1)}
	w1 := &fakeNotifyWatcher{changes: make(chan struct{}, 1)}
	w0.changes <- struct{}{}
	w1.changes <- struct{}{}

	mw := common.NewMultiNotifyWatcher(w0, w1)
	defer statetesting.AssertStop(c, mw)

	wc := statetesting.NewNotifyWatcherC(c, nopSyncStarter{}, mw)
	wc.AssertOneChange()

	w0.changes <- struct{}{}
	wc.AssertOneChange()
	w1.changes <- struct{}{}
	wc.AssertOneChange()

	w0.changes <- struct{}{}
	w1.changes <- struct{}{}
	wc.AssertOneChange()
}

func (*multiNotifyWatcherSuite) TestMultiNotifyWatcherStop(c *gc.C) {
	w0 := &fakeNotifyWatcher{changes: make(chan struct{}, 1)}
	w1 := &fakeNotifyWatcher{changes: make(chan struct{}, 1)}
	w0.changes <- struct{}{}
	w1.changes <- struct{}{}

	mw := common.NewMultiNotifyWatcher(w0, w1)
	wc := statetesting.NewNotifyWatcherC(c, nopSyncStarter{}, mw)
	wc.AssertOneChange()
	statetesting.AssertCanStopWhenSending(c, mw)
	wc.AssertClosed()
}

type nopSyncStarter struct{}

func (nopSyncStarter) StartSync() {}
