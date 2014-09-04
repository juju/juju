// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package firewaller_test

import (
	"fmt"

	"github.com/juju/names"
	gc "launchpad.net/gocheck"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/firewaller"
	"github.com/juju/juju/apiserver/params"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing"
)

type portsWatcherSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&portsWatcherSuite{})

type fakePortsWatcher struct {
	state.PortsWatcher
	initial []string
}

func (f *fakePortsWatcher) WatchOpenedPorts() state.StringsWatcher {
	changes := make(chan []string, 1)
	changes <- f.initial
	return &apiservertesting.FakeStringsWatcher{changes}
}

func (s *portsWatcherSuite) TestWatchSuccess(c *gc.C) {
	getCanWatch := func() (common.AuthFunc, error) {
		return func(tag names.Tag) bool {
			return true
		}, nil
	}
	resources := common.NewResources()
	s.AddCleanup(func(_ *gc.C) { resources.StopAll() })
	p := firewaller.NewOpenedPortsWatcher(
		&fakePortsWatcher{},
		resources,
		getCanWatch,
	)
	result, err := p.WatchOpenedPorts(params.Entities{[]params.Entity{{""}}})
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(err, gc.IsNil)
	c.Assert(result.Results[0], gc.DeepEquals, params.StringsWatchResult{StringsWatcherId: "1", Changes: nil, Error: nil})
	c.Assert(resources.Count(), gc.Equals, 1)
}

func (s *portsWatcherSuite) TestWatchGetAuthError(c *gc.C) {
	getCanWatch := func() (common.AuthFunc, error) {
		return nil, fmt.Errorf("pow")
	}
	resources := common.NewResources()
	s.AddCleanup(func(_ *gc.C) { resources.StopAll() })
	p := firewaller.NewOpenedPortsWatcher(
		&fakePortsWatcher{},
		resources,
		getCanWatch,
	)
	result, err := p.WatchOpenedPorts(params.Entities{[]params.Entity{{""}}})
	c.Assert(err, gc.IsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0].Error, gc.ErrorMatches, "pow")
	c.Assert(resources.Count(), gc.Equals, 0)
}

func (s *portsWatcherSuite) TestWatchAuthError(c *gc.C) {
	getCanWatch := func() (common.AuthFunc, error) {
		return func(tag names.Tag) bool {
			return false
		}, nil
	}
	resources := common.NewResources()
	s.AddCleanup(func(_ *gc.C) { resources.StopAll() })
	p := firewaller.NewOpenedPortsWatcher(
		&fakePortsWatcher{},
		resources,
		getCanWatch,
	)
	result, err := p.WatchOpenedPorts(params.Entities{[]params.Entity{{"environment-573cfc28-5c4b-4684-9259-9573a39dc314"}}})
	c.Assert(err, gc.IsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0].Error, gc.ErrorMatches, "permission denied")
	c.Assert(resources.Count(), gc.Equals, 0)
}
