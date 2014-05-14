// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"fmt"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/apiserver/common"
	apiservertesting "launchpad.net/juju-core/state/apiserver/testing"
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
	return &fakeNotifyWatcher{changes}
}

type fakeNotifyWatcher struct {
	changes chan struct{}
}

func (*fakeNotifyWatcher) Stop() error {
	return nil
}

func (*fakeNotifyWatcher) Kill() {}

func (*fakeNotifyWatcher) Wait() error {
	return nil
}

func (*fakeNotifyWatcher) Err() error {
	return nil
}

func (w *fakeNotifyWatcher) Changes() <-chan struct{} {
	return w.changes
}

func (*agentEntityWatcherSuite) TestWatch(c *gc.C) {
	st := &fakeState{
		entities: map[string]entityWithError{
			"x0": &fakeAgentEntityWatcher{fetchError: "x0 fails"},
			"x1": &fakeAgentEntityWatcher{},
			"x2": &fakeAgentEntityWatcher{},
		},
	}
	getCanWatch := func() (common.AuthFunc, error) {
		return func(tag string) bool {
			switch tag {
			case "x0", "x1":
				return true
			}
			return false
		}, nil
	}
	resources := common.NewResources()
	a := common.NewAgentEntityWatcher(st, resources, getCanWatch)
	entities := params.Entities{[]params.Entity{
		{"x0"}, {"x1"}, {"x2"}, {"x3"},
	}}
	result, err := a.Watch(entities)
	c.Assert(err, gc.IsNil)
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
	c.Assert(err, gc.IsNil)
	c.Assert(result.Results, gc.HasLen, 0)
}
