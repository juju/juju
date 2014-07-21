// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"fmt"

	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "launchpad.net/gocheck"

	"github.com/juju/juju/state"
	"github.com/juju/juju/state/api/params"
	"github.com/juju/juju/state/apiserver/common"
	"github.com/juju/juju/testing"
)

type environMachinesWatcherSuite struct {
	testing.BaseSuite
}

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

func (s *environMachinesWatcherSuite) TestWatchEnvironMachines(c *gc.C) {
	getCanWatch := func() (common.AuthFunc, error) {
		return func(tag names.Tag) bool {
			return true
		}, nil
	}
	resources := common.NewResources()
	s.AddCleanup(func(_ *gc.C) { resources.StopAll() })
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

func (s *environMachinesWatcherSuite) TestWatchGetAuthError(c *gc.C) {
	getCanWatch := func() (common.AuthFunc, error) {
		return nil, fmt.Errorf("pow")
	}
	resources := common.NewResources()
	s.AddCleanup(func(_ *gc.C) { resources.StopAll() })
	e := common.NewEnvironMachinesWatcher(
		&fakeEnvironMachinesWatcher{},
		resources,
		getCanWatch,
	)
	_, err := e.WatchEnvironMachines()
	c.Assert(err, gc.ErrorMatches, "pow")
	c.Assert(resources.Count(), gc.Equals, 0)
}

func (s *environMachinesWatcherSuite) TestWatchAuthError(c *gc.C) {
	getCanWatch := func() (common.AuthFunc, error) {
		return func(tag names.Tag) bool {
			return false
		}, nil
	}
	resources := common.NewResources()
	s.AddCleanup(func(_ *gc.C) { resources.StopAll() })
	e := common.NewEnvironMachinesWatcher(
		&fakeEnvironMachinesWatcher{},
		resources,
		getCanWatch,
	)
	_, err := e.WatchEnvironMachines()
	c.Assert(err, gc.ErrorMatches, "permission denied")
	c.Assert(resources.Count(), gc.Equals, 0)
}
