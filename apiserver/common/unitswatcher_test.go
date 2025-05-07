// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"context"
	"fmt"

	"github.com/juju/names/v6"
	"github.com/juju/tc"

	"github.com/juju/juju/apiserver/common"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

type unitsWatcherSuite struct{}

var _ = tc.Suite(&unitsWatcherSuite{})

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

func (*unitsWatcherSuite) TestWatchUnits(c *tc.C) {
	st := &fakeState{
		entities: map[names.Tag]entityWithError{
			u("x/0"): &fakeUnitsWatcher{fetchError: "x0 fails"},
			u("x/1"): &fakeUnitsWatcher{initial: []string{"foo", "bar"}},
			u("x/2"): &fakeUnitsWatcher{},
		},
	}
	getCanWatch := func(ctx context.Context) (common.AuthFunc, error) {
		x0 := u("x/0")
		x1 := u("x/1")
		return func(tag names.Tag) bool {
			return tag == x0 || tag == x1
		}, nil
	}
	resources := common.NewResources()
	w := common.NewUnitsWatcher(st, resources, getCanWatch)
	entities := params.Entities{Entities: []params.Entity{
		{Tag: "unit-x-0"}, {Tag: "unit-x-1"}, {Tag: "unit-x-2"}, {Tag: "unit-x-3"},
	}}
	result, err := w.WatchUnits(context.Background(), entities)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.StringsWatchResults{
		Results: []params.StringsWatchResult{
			{Error: &params.Error{Message: "x0 fails"}},
			{StringsWatcherId: "1", Changes: []string{"foo", "bar"}, Error: nil},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})
}

func (*unitsWatcherSuite) TestWatchUnitsError(c *tc.C) {
	getCanWatch := func(ctx context.Context) (common.AuthFunc, error) {
		return nil, fmt.Errorf("pow")
	}
	resources := common.NewResources()
	w := common.NewUnitsWatcher(
		&fakeState{},
		resources,
		getCanWatch,
	)
	_, err := w.WatchUnits(context.Background(), params.Entities{Entities: []params.Entity{{Tag: "x0"}}})
	c.Assert(err, tc.ErrorMatches, "pow")
}

func (*unitsWatcherSuite) TestWatchNoArgsNoError(c *tc.C) {
	getCanWatch := func(ctx context.Context) (common.AuthFunc, error) {
		return nil, fmt.Errorf("pow")
	}
	resources := common.NewResources()
	w := common.NewUnitsWatcher(
		&fakeState{},
		resources,
		getCanWatch,
	)
	result, err := w.WatchUnits(context.Background(), params.Entities{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.HasLen, 0)
}
