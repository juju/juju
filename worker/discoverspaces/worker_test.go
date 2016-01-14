// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package discoverspaces_test

import (
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/set"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api"
	apidiscoverspaces "github.com/juju/juju/api/discoverspaces"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/network"
	"github.com/juju/juju/provider/common"
	"github.com/juju/juju/provider/dummy"
	"github.com/juju/juju/state"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/discoverspaces"
)

type workerSuite struct {
	testing.JujuConnSuite

	Worker  worker.Worker
	OpsChan chan dummy.Operation

	APIConnection api.Connection
	API           *apidiscoverspaces.API
}

var _ = gc.Suite(&workerSuite{})

func (s *workerSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)

	// Unbreak dummy provider methods.
	s.AssertConfigParameterUpdated(c, "broken", "")

	s.APIConnection, _ = s.OpenAPIAsNewMachine(c, state.JobManageEnviron)
	s.API = s.APIConnection.DiscoverSpaces()

	//s.State.StartSync()

	s.OpsChan = make(chan dummy.Operation, 10)
	dummy.Listen(s.OpsChan)
}

func (s *workerSuite) startWorker() {
	s.Worker = discoverspaces.NewWorker(s.API)
}

func (s *workerSuite) TearDownTest(c *gc.C) {
	if s.Worker != nil {
		c.Assert(worker.Stop(s.Worker), jc.ErrorIsNil)
	}
	s.JujuConnSuite.TearDownTest(c)
}

func (s *workerSuite) TestConvertSpaceName(c *gc.C) {
	empty := set.Strings{}
	nameTests := []struct {
		name     string
		existing set.Strings
		expected string
	}{
		{"foo", empty, "foo"},
		{"foo1", empty, "foo1"},
		{"Foo Thing", empty, "foo-thing"},
		{"foo^9*//++!!!!", empty, "foo9"},
		{"--Foo", empty, "foo"},
		{"---^^&*()!", empty, "empty"},
		{" ", empty, "empty"},
		{"", empty, "empty"},
		{"foo\u2318", empty, "foo"},
		{"foo--", empty, "foo"},
		{"-foo--foo----bar-", empty, "foo-foo-bar"},
		{"foo-", set.NewStrings("foo", "bar", "baz"), "foo-2"},
		{"foo", set.NewStrings("foo", "foo-2"), "foo-3"},
		{"---", set.NewStrings("empty"), "empty-2"},
	}
	for _, test := range nameTests {
		result := discoverspaces.ConvertSpaceName(test.name, test.existing)
		c.Check(result, gc.Equals, test.expected)
	}
}

func (s *workerSuite) TestWorkerIsStringsWorker(c *gc.C) {
	s.startWorker()
	c.Assert(s.Worker, gc.Not(gc.FitsTypeOf), worker.FinishedWorker{})
}

func (s *workerSuite) TestWorkerSupportsSpaceDiscoveryFalse(c *gc.C) {
	s.startWorker()
	spaces, err := s.State.AllSpaces()
	c.Assert(err, jc.ErrorIsNil)

	// No spaces will have been created, worker does nothing.
	for a := common.ShortAttempt.Start(); a.Next(); {
		spaces, err = s.State.AllSpaces()
		if err != nil {
			c.Fatalf("error fetching spaces: %v", err)
		}
		if len(spaces) != 0 {
			c.Fatalf("spaces should not be created, we have %v", len(spaces))
		}
		if !a.HasNext() {
			break
		}
	}
}

func (s *workerSuite) TestWorkerDiscoversSpaces(c *gc.C) {
	dummy.SetSupportsSpaceDiscovery(true)
	s.startWorker()
	var err error
	var spaces []*state.Space
	for a := common.ShortAttempt.Start(); a.Next(); {
		spaces, err = s.State.AllSpaces()
		if err != nil {
			break
		}
		if len(spaces) == 4 {
			// All spaces have been created.
			break
		}
		if !a.HasNext() {
			c.Fatalf("spaces not imported")
		}
	}
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(spaces, gc.HasLen, 4)
	expectedSpaces := []network.SpaceInfo{{
		Name:       "foo",
		ProviderId: network.Id("foo"),
		Subnets: []network.SubnetInfo{
			{
				ProviderId:        network.Id("1"),
				CIDR:              "192.168.1.0/24",
				AvailabilityZones: []string{"zone1"},
			}, {
				ProviderId:        network.Id("2"),
				CIDR:              "192.168.2.0/24",
				AvailabilityZones: []string{"zone1"},
			}}}, {
		Name:       "another-foo-99",
		ProviderId: network.Id("Another Foo 99!"),
		Subnets: []network.SubnetInfo{
			{
				ProviderId:        network.Id("3"),
				CIDR:              "192.168.3.0/24",
				AvailabilityZones: []string{"zone1"},
			}}}, {
		Name:       "foo-2",
		ProviderId: network.Id("foo-"),
		Subnets: []network.SubnetInfo{
			{
				ProviderId:        network.Id("4"),
				CIDR:              "192.168.4.0/24",
				AvailabilityZones: []string{"zone1"},
			}}}, {
		Name:       "empty",
		ProviderId: network.Id("---"),
		Subnets: []network.SubnetInfo{
			{
				ProviderId:        network.Id("5"),
				CIDR:              "192.168.5.0/24",
				AvailabilityZones: []string{"zone1"},
			}}}}
	expectedSpaceMap := make(map[string]network.SpaceInfo)
	for _, space := range expectedSpaces {
		expectedSpaceMap[space.Name] = space
	}
	for _, space := range spaces {
		expected, ok := expectedSpaceMap[space.Name()]
		c.Assert(ok, jc.IsTrue)
		c.Assert(space.ProviderId(), gc.Equals, expected.ProviderId)
		subnets, err := space.Subnets()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(len(subnets), gc.Equals, len(expected.Subnets))
		for i, subnet := range subnets {
			expectedSubnet := expected.Subnets[i]
			c.Assert(subnet.ProviderId(), gc.Equals, expectedSubnet.ProviderId)
			c.Assert([]string{subnet.AvailabilityZone()}, jc.DeepEquals, expectedSubnet.AvailabilityZones)
			c.Assert(subnet.CIDR(), gc.Equals, expectedSubnet.CIDR)
		}
	}
}
