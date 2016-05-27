// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/state"
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
	authorizer := apiservertesting.FakeAuthorizer{
		Tag:            names.NewMachineTag("0"),
		EnvironManager: true,
	}
	resources := common.NewResources()
	s.AddCleanup(func(_ *gc.C) { resources.StopAll() })
	e := common.NewEnvironMachinesWatcher(
		&fakeEnvironMachinesWatcher{initial: []string{"foo"}},
		resources,
		authorizer,
	)
	result, err := e.WatchEnvironMachines()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, params.StringsWatchResult{"1", []string{"foo"}, nil})
	c.Assert(resources.Count(), gc.Equals, 1)
}

func (s *environMachinesWatcherSuite) TestWatchAuthError(c *gc.C) {
	authorizer := apiservertesting.FakeAuthorizer{
		Tag:            names.NewMachineTag("1"),
		EnvironManager: false,
	}
	resources := common.NewResources()
	s.AddCleanup(func(_ *gc.C) { resources.StopAll() })
	e := common.NewEnvironMachinesWatcher(
		&fakeEnvironMachinesWatcher{},
		resources,
		authorizer,
	)
	_, err := e.WatchEnvironMachines()
	c.Assert(err, gc.ErrorMatches, "permission denied")
	c.Assert(resources.Count(), gc.Equals, 0)
}
