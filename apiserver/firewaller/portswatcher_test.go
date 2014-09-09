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
	fakeEnvTag := names.NewEnvironTag("39ed2071-2a81-40c3-bb76-658c7b8469f9")
	getCanWatch := apiservertesting.FakeAuthFunc([]names.Tag{fakeEnvTag})
	resources := common.NewResources()
	s.AddCleanup(func(_ *gc.C) { resources.StopAll() })
	p := firewaller.NewOpenedPortsWatcherFactory(
		&fakePortsWatcher{},
		resources,
		getCanWatch,
	)

	result, err := p.WatchOpenedPorts(params.Entities{[]params.Entity{{fakeEnvTag.String()}}})
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(err, gc.IsNil)
	c.Assert(result.Results[0].Error, gc.IsNil)
	c.Assert(result.Results[0].Changes, gc.HasLen, 0)
	c.Assert(resources.Count(), gc.Equals, 1)

}

func (s *portsWatcherSuite) TestWatchGetAuthError(c *gc.C) {
	getCanWatch := func() (common.AuthFunc, error) {
		return nil, fmt.Errorf("pow")
	}
	resources := common.NewResources()
	s.AddCleanup(func(_ *gc.C) { resources.StopAll() })
	p := firewaller.NewOpenedPortsWatcherFactory(
		&fakePortsWatcher{},
		resources,
		getCanWatch,
	)
	result, err := p.WatchOpenedPorts(params.Entities{[]params.Entity{{""}}})
	c.Assert(err, gc.ErrorMatches, "pow")
	c.Assert(result.Results, gc.HasLen, 0)
}

func (s *portsWatcherSuite) TestWatchAuthError(c *gc.C) {
	getCanWatch := apiservertesting.FakeAuthFunc(nil)
	resources := common.NewResources()
	s.AddCleanup(func(_ *gc.C) { resources.StopAll() })
	p := firewaller.NewOpenedPortsWatcherFactory(
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

func (s *portsWatcherSuite) TestWatchBadEntities(c *gc.C) {
	fakeEnvTag := names.NewEnvironTag("39ed2071-2a81-40c3-bb76-658c7b8469f9")
	getCanWatch := apiservertesting.FakeAuthFunc([]names.Tag{fakeEnvTag})
	resources := common.NewResources()
	s.AddCleanup(func(_ *gc.C) { resources.StopAll() })
	p := firewaller.NewOpenedPortsWatcherFactory(
		&fakePortsWatcher{},
		resources,
		getCanWatch,
	)

	entities := params.Entities{[]params.Entity{
		{fakeEnvTag.String()},
		{names.NewUnitTag("x/0").String()},
		{names.NewMachineTag("0").String()},
		{names.NewEnvironTag("11111111-1111-1111-1111-111111111111").String()},
		{"total-gibberish"},
	}}

	result, err := p.WatchOpenedPorts(entities)
	c.Assert(result.Results, gc.HasLen, len(entities.Entities))
	c.Assert(err, gc.IsNil)
	// check result for first entity
	c.Assert(result.Results[0].Error, gc.IsNil)
	c.Assert(result.Results[0], gc.DeepEquals, params.StringsWatchResult{StringsWatcherId: "1", Changes: nil, Error: nil})
	// check invalid entities
	for _, res := range result.Results[1:] {
		c.Assert(res.Error, gc.NotNil)
	}
	c.Assert(resources.Count(), gc.Equals, 1)
}
