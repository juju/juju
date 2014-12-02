// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"fmt"

	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/state"
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
	resources := common.NewResources()
	w := common.NewUnitsWatcher(st, resources, getCanWatch)
	entities := params.Entities{[]params.Entity{
		{"unit-x-0"}, {"unit-x-1"}, {"unit-x-2"}, {"unit-x-3"},
	}}
	result, err := w.WatchUnits(entities)
	c.Assert(err, jc.ErrorIsNil)
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
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 0)
}
