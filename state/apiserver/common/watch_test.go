// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"fmt"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/errors"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/apiserver/common"
	apiservertesting "launchpad.net/juju-core/state/apiserver/testing"
)

type agentEntityWatcherSuite struct{}

var _ = gc.Suite(&agentEntityWatcherSuite{})

type fakeAgentEntityWatcherState struct {
	entities map[string]*fakeAgentEntityWatcher
}

func (st *fakeAgentEntityWatcherState) AgentEntityWatcher(tag string) (state.AgentEntityWatcher, error) {
	if agentEntityWatcher, ok := st.entities[tag]; ok {
		if agentEntityWatcher.err != nil {
			return nil, agentEntityWatcher.err
		}
		return agentEntityWatcher, nil
	}
	return nil, errors.NotFoundf("entity %q", tag)
}

type fakeAgentEntityWatcher struct {
	err error
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

func (*fakeNotifyWatcher) Err() error {
	return nil
}

func (w *fakeNotifyWatcher) Changes() <-chan struct{} {
	return w.changes
}

func (*agentEntityWatcherSuite) TestWatch(c *gc.C) {
	st := &fakeAgentEntityWatcherState{
		entities: map[string]*fakeAgentEntityWatcher{
			"x0": {err: fmt.Errorf("x0 fails")},
			"x1": {},
			"x2": {},
			"x3": {err: fmt.Errorf("x3 error")},
		},
	}
	getCanWatch := func() (common.AuthFunc, error) {
		return func(tag string) bool {
			switch tag {
			case "x0", "x1", "x3":
				return true
			}
			return false
		}, nil
	}
	resources := common.NewResources()
	a := common.NewAgentEntityWatcher(st, resources, getCanWatch)
	entities := params.Entities{[]params.Entity{
		{"x0"}, {"x1"}, {"x2"}, {"x3"}, {"x4"},
	}}
	result, err := a.Watch(entities)
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.DeepEquals, params.NotifyWatchResults{
		Results: []params.NotifyWatchResult{
			{Error: &params.Error{Message: "x0 fails"}},
			{"1", nil},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: &params.Error{Message: "x3 error"}},
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
		&fakeAgentEntityWatcherState{},
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
		&fakeAgentEntityWatcherState{},
		resources,
		getCanWatch,
	)
	result, err := a.Watch(params.Entities{})
	c.Assert(err, gc.IsNil)
	c.Assert(result.Results, gc.HasLen, 0)
}
