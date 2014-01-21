// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"fmt"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/apiserver/common"
	jc "launchpad.net/juju-core/testing/checkers"
)

type environMachinesWatcherSuite struct{}

var _ = gc.Suite(&environMachinesWatcherSuite{})

type fakeEnvironMachinesWatcher struct {
	state.EnvironMachinesWatcher
	initial []string
}

func (f *fakeEnvironMachinesWatcher) WatchEnvironMachines() state.StringsWatcher {
	changes := make(chan []string, 1)
	// Simulate initial event.
	changes <- f.initial
	return &fakeStringsWatcher{changes}
}

func (*environMachinesWatcherSuite) TestWatchEnvironMachines(c *gc.C) {
	getCanWatch := func() (common.AuthFunc, error) {
		return func(tag string) bool {
			return true
		}, nil
	}
	resources := common.NewResources()
	e := common.NewEnvironMachinesWatcher(
		&fakeEnvironMachinesWatcher{initial: []string{"foo"}},
		resources,
		getCanWatch,
	)
	result, err := e.WatchEnvironMachines()
	c.Assert(err, gc.IsNil)
	c.Assert(result, jc.DeepEquals, params.StringsWatchResult{"1", []string{"foo"}, nil})
	c.Assert(resources.Count(), gc.Equals, 1)
}

func (*environMachinesWatcherSuite) TestWatchGetAuthError(c *gc.C) {
	getCanWatch := func() (common.AuthFunc, error) {
		return nil, fmt.Errorf("pow")
	}
	resources := common.NewResources()
	e := common.NewEnvironMachinesWatcher(
		&fakeEnvironMachinesWatcher{},
		resources,
		getCanWatch,
	)
	_, err := e.WatchEnvironMachines()
	c.Assert(err, gc.ErrorMatches, "pow")
	c.Assert(resources.Count(), gc.Equals, 0)
}

func (*environMachinesWatcherSuite) TestWatchAuthError(c *gc.C) {
	getCanWatch := func() (common.AuthFunc, error) {
		return func(tag string) bool {
			return false
		}, nil
	}
	resources := common.NewResources()
	e := common.NewEnvironMachinesWatcher(
		&fakeEnvironMachinesWatcher{},
		resources,
		getCanWatch,
	)
	_, err := e.WatchEnvironMachines()
	c.Assert(err, gc.ErrorMatches, "permission denied")
	c.Assert(resources.Count(), gc.Equals, 0)
}
