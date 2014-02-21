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
	jc "launchpad.net/juju-core/testing/checkers"
)

type unitsWatcherSuite struct{}

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

func (*unitsWatcherSuite) TestWatchUnits(c *gc.C) {
	st := &fakeState{
		entities: map[string]entityWithError{
			"x0": &fakeUnitsWatcher{fetchError: "x0 fails"},
			"x1": &fakeUnitsWatcher{initial: []string{"foo", "bar"}},
			"x2": &fakeUnitsWatcher{},
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
	w := common.NewUnitsWatcher(st, resources, getCanWatch)
	entities := params.Entities{[]params.Entity{
		{"x0"}, {"x1"}, {"x2"}, {"x3"},
	}}
	result, err := w.WatchUnits(entities)
	c.Assert(err, gc.IsNil)
	c.Assert(result, jc.DeepEquals, params.StringsWatchResults{
		Results: []params.StringsWatchResult{
			{Error: &params.Error{Message: "x0 fails"}},
			{"1", []string{"foo", "bar"}, nil},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})
}

func (*unitsWatcherSuite) TestWatchUnitsError(c *gc.C) {
	getCanWatch := func() (common.AuthFunc, error) {
		return nil, fmt.Errorf("pow")
	}
	resources := common.NewResources()
	w := common.NewUnitsWatcher(
		&fakeState{},
		resources,
		getCanWatch,
	)
	_, err := w.WatchUnits(params.Entities{[]params.Entity{{"x0"}}})
	c.Assert(err, gc.ErrorMatches, "pow")
}

func (*unitsWatcherSuite) TestWatchNoArgsNoError(c *gc.C) {
	getCanWatch := func() (common.AuthFunc, error) {
		return nil, fmt.Errorf("pow")
	}
	resources := common.NewResources()
	w := common.NewUnitsWatcher(
		&fakeState{},
		resources,
		getCanWatch,
	)
	result, err := w.WatchUnits(params.Entities{})
	c.Assert(err, gc.IsNil)
	c.Assert(result.Results, gc.HasLen, 0)
}
