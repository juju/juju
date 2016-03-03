// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package discoverspaces_test

import (
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/set"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api"
	apidiscoverspaces "github.com/juju/juju/api/discoverspaces"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/environs"
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

	APIConnection    api.Connection
	API              *apidiscoverspaces.API
	spacesDiscovered chan struct{}
}

var _ = gc.Suite(&workerSuite{})

func (s *workerSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)

	// Unbreak dummy provider methods.
	s.AssertConfigParameterUpdated(c, "broken", "")

	s.APIConnection, _ = s.OpenAPIAsNewMachine(c, state.JobManageModel)
	s.API = s.APIConnection.DiscoverSpaces()

	s.OpsChan = make(chan dummy.Operation, 10)
	dummy.Listen(s.OpsChan)
	s.spacesDiscovered = nil
}

func (s *workerSuite) startWorker() {
	s.Worker, s.spacesDiscovered = discoverspaces.NewWorker(s.API)
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

func (s *workerSuite) assertSpaceDiscoveryCompleted(c *gc.C) {
	c.Assert(s.spacesDiscovered, gc.NotNil)
	select {
	case <-s.spacesDiscovered:
		// The channel was closed as it should be
		return
	default:
		c.Fatalf("Space discovery channel not closed")
	}
}

func (s *workerSuite) TestWorkerSupportsNetworkingFalse(c *gc.C) {
	// We set SupportsSpaceDiscovery to true so that spaces *would* be
	// discovered if networking was supported. So we know that if they're
	// discovered it must be because networking is not supported.
	dummy.SetSupportsSpaceDiscovery(true)
	noNetworking := func(environs.Environ) (environs.NetworkingEnviron, bool) {
		return nil, false
	}
	s.PatchValue(&environs.SupportsNetworking, noNetworking)
	s.startWorker()

	// No spaces will have been created, worker does nothing.
	for a := common.ShortAttempt.Start(); a.Next(); {
		spaces, err := s.State.AllSpaces()
		c.Assert(err, jc.ErrorIsNil)
		if len(spaces) != 0 {
			c.Fatalf("spaces should not be created, we have %v", len(spaces))
		}
		if !a.HasNext() {
			break
		}
	}
	s.assertSpaceDiscoveryCompleted(c)
}

func (s *workerSuite) TestWorkerSupportsSpaceDiscoveryFalse(c *gc.C) {
	s.startWorker()

	// No spaces will have been created, worker does nothing.
	for a := common.ShortAttempt.Start(); a.Next(); {
		spaces, err := s.State.AllSpaces()
		c.Assert(err, jc.ErrorIsNil)
		if len(spaces) != 0 {
			c.Fatalf("spaces should not be created, we have %v", len(spaces))
		}
		if !a.HasNext() {
			break
		}
	}
	s.assertSpaceDiscoveryCompleted(c)
}

func (s *workerSuite) TestWorkerDiscoversSpaces(c *gc.C) {
	dummy.SetSupportsSpaceDiscovery(true)
	s.startWorker()
	for a := common.ShortAttempt.Start(); a.Next(); {
		var found bool
		select {
		case <-s.spacesDiscovered:
			// The channel was closed so discovery has completed.
			found = true
		}
		if found {
			break
		}
		if !a.HasNext() {
			c.Fatalf("discovery not completed")
		}
	}

	spaces, err := s.State.AllSpaces()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(spaces, gc.HasLen, 4)
	expectedSpaces := []network.SpaceInfo{{
		Name:       "foo",
		ProviderId: network.Id("foo"),
		Subnets: []network.SubnetInfo{{
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
		Subnets: []network.SubnetInfo{{
			ProviderId:        network.Id("3"),
			CIDR:              "192.168.3.0/24",
			AvailabilityZones: []string{"zone1"},
		}}}, {
		Name:       "foo-2",
		ProviderId: network.Id("foo-"),
		Subnets: []network.SubnetInfo{{
			ProviderId:        network.Id("4"),
			CIDR:              "192.168.4.0/24",
			AvailabilityZones: []string{"zone1"},
		}}}, {
		Name:       "empty",
		ProviderId: network.Id("---"),
		Subnets: []network.SubnetInfo{{
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
		if !c.Check(ok, jc.IsTrue) {
			continue
		}
		c.Check(space.ProviderId(), gc.Equals, expected.ProviderId)
		subnets, err := space.Subnets()
		if !c.Check(err, jc.ErrorIsNil) {
			continue
		}
		if !c.Check(len(subnets), gc.Equals, len(expected.Subnets)) {
			continue
		}
		for i, subnet := range subnets {
			expectedSubnet := expected.Subnets[i]
			c.Check(subnet.ProviderId(), gc.Equals, expectedSubnet.ProviderId)
			c.Check([]string{subnet.AvailabilityZone()}, jc.DeepEquals, expectedSubnet.AvailabilityZones)
			c.Check(subnet.CIDR(), gc.Equals, expectedSubnet.CIDR)
		}
	}
	s.assertSpaceDiscoveryCompleted(c)
}

func (s *workerSuite) TestWorkerIdempotent(c *gc.C) {
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
	newWorker, _ := discoverspaces.NewWorker(s.API)

	// This ensures that the worker can handle re-importing without error.
	defer func() {
		c.Assert(worker.Stop(newWorker), jc.ErrorIsNil)
	}()

	// Check that no extra spaces are imported.
	for a := common.ShortAttempt.Start(); a.Next(); {
		spaces, err = s.State.AllSpaces()
		if err != nil {
			break
		}
		if len(spaces) != 4 {
			c.Fatalf("unexpected number of spaces: %v", len(spaces))
		}
		if !a.HasNext() {
			break
		}
	}
}

func (s *workerSuite) TestSupportsSpaceDiscoveryBroken(c *gc.C) {
	s.AssertConfigParameterUpdated(c, "broken", "SupportsSpaceDiscovery")

	newWorker, spacesDiscovered := discoverspaces.NewWorker(s.API)
	s.spacesDiscovered = spacesDiscovered
	err := worker.Stop(newWorker)
	c.Assert(err, gc.ErrorMatches, "dummy.SupportsSpaceDiscovery is broken")
	s.assertSpaceDiscoveryCompleted(c)
}

func (s *workerSuite) TestSpacesBroken(c *gc.C) {
	dummy.SetSupportsSpaceDiscovery(true)
	s.AssertConfigParameterUpdated(c, "broken", "Spaces")

	newWorker, spacesDiscovered := discoverspaces.NewWorker(s.API)
	s.spacesDiscovered = spacesDiscovered
	err := worker.Stop(newWorker)
	c.Assert(err, gc.ErrorMatches, "dummy.Spaces is broken")
	s.assertSpaceDiscoveryCompleted(c)
}

func (s *workerSuite) TestWorkerIgnoresExistingSpacesAndSubnets(c *gc.C) {
	dummy.SetSupportsSpaceDiscovery(true)
	spaceTag := names.NewSpaceTag("foo")
	args := params.CreateSpacesParams{
		Spaces: []params.CreateSpaceParams{{
			Public:     false,
			SpaceTag:   spaceTag.String(),
			ProviderId: "foo",
		}}}
	result, err := s.API.CreateSpaces(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0].Error, gc.IsNil)

	subnetArgs := params.AddSubnetsParams{
		Subnets: []params.AddSubnetParams{{
			SubnetProviderId: "1",
			SpaceTag:         spaceTag.String(),
			Zones:            []string{"zone1"},
		}}}
	subnetResult, err := s.API.AddSubnets(subnetArgs)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(subnetResult.Results, gc.HasLen, 1)
	c.Assert(subnetResult.Results[0].Error, gc.IsNil)

	s.startWorker()
	for a := common.ShortAttempt.Start(); a.Next(); {
		spaces, err := s.State.AllSpaces()
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
}
