// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package discoverspaces_test

import (
	"sync/atomic"
	"time"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/api"
	apidiscoverspaces "github.com/juju/juju/api/discoverspaces"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/network"
	"github.com/juju/juju/provider/dummy"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/discoverspaces"
	"github.com/juju/juju/worker/gate"
	"github.com/juju/juju/worker/workertest"
)

type WorkerSuite struct {
	// TODO(fwereade): we *really* should not be using
	// JujuConnSuite in new code.
	testing.JujuConnSuite

	APIConnection api.Connection
	API           *checkingFacade

	numCreateSpaceCalls uint32
	numAddSubnetsCalls  uint32
}

type checkingFacade struct {
	apidiscoverspaces.API

	createSpacesCallback func()
	addSubnetsCallback   func()
}

func (cf *checkingFacade) CreateSpaces(args params.CreateSpacesParams) (results params.ErrorResults, err error) {
	if cf.createSpacesCallback != nil {
		cf.createSpacesCallback()
	}
	return cf.API.CreateSpaces(args)
}

func (cf *checkingFacade) AddSubnets(args params.AddSubnetsParams) (params.ErrorResults, error) {
	if cf.addSubnetsCallback != nil {
		cf.addSubnetsCallback()
	}
	return cf.API.AddSubnets(args)
}

var _ = gc.Suite(&WorkerSuite{})

func (s *WorkerSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)

	// Unbreak dummy provider methods.
	s.AssertConfigParameterUpdated(c, "broken", "")

	s.APIConnection, _ = s.OpenAPIAsNewMachine(c, state.JobManageModel)

	realAPI := s.APIConnection.DiscoverSpaces()
	s.API = &checkingFacade{
		API: *realAPI,
		createSpacesCallback: func() {
			atomic.AddUint32(&s.numCreateSpaceCalls, 1)
		},
		addSubnetsCallback: func() {
			atomic.AddUint32(&s.numAddSubnetsCalls, 1)
		},
	}
}

func (s *WorkerSuite) TearDownTest(c *gc.C) {
	if s.APIConnection != nil {
		c.Check(s.APIConnection.Close(), jc.ErrorIsNil)
	}
	s.JujuConnSuite.TearDownTest(c)
}

func (s *WorkerSuite) TestSupportsSpaceDiscoveryBroken(c *gc.C) {
	s.AssertConfigParameterUpdated(c, "broken", "SupportsSpaceDiscovery")

	worker, lock := s.startWorker(c)
	err := workertest.CheckKilled(c, worker)
	c.Assert(err, gc.ErrorMatches, "dummy.SupportsSpaceDiscovery is broken")

	select {
	case <-time.After(coretesting.ShortWait):
	case <-lock.Unlocked():
		c.Fatalf("gate unlocked despite worker failure")
	}
}

func (s *WorkerSuite) TestSpacesBroken(c *gc.C) {
	dummy.SetSupportsSpaceDiscovery(true)
	s.AssertConfigParameterUpdated(c, "broken", "Spaces")

	worker, lock := s.startWorker(c)
	err := workertest.CheckKilled(c, worker)
	c.Assert(err, gc.ErrorMatches, "dummy.Spaces is broken")

	select {
	case <-time.After(coretesting.ShortWait):
	case <-lock.Unlocked():
		c.Fatalf("gate unlocked despite worker failure")
	}
}

func (s *WorkerSuite) TestWorkerSupportsNetworkingFalse(c *gc.C) {
	// We set SupportsSpaceDiscovery to true so that spaces *would* be
	// discovered if networking was supported. So we know that if they're
	// discovered it must be because networking is not supported.
	dummy.SetSupportsSpaceDiscovery(true)

	// TODO(fwereade): monkey-patching remote packages is even worse
	// than monkey-patching local packages, please don't do it.
	noNetworking := func(environs.Environ) (environs.NetworkingEnviron, bool) {
		return nil, false
	}
	s.PatchValue(&environs.SupportsNetworking, noNetworking)

	s.unlockCheck(c, s.assertDiscoveredNoSpaces)
}

func (s *WorkerSuite) TestWorkerSupportsSpaceDiscoveryFalse(c *gc.C) {
	s.unlockCheck(c, s.assertDiscoveredNoSpaces)
}

func (s *WorkerSuite) TestWorkerDiscoversSpaces(c *gc.C) {
	dummy.SetSupportsSpaceDiscovery(true)
	s.unlockCheck(c, func(*gc.C) {
		s.assertDiscoveredSpaces(c)
		s.assertNumCalls(c, 1, 1)
	})
}

func (s *WorkerSuite) TestWorkerIdempotent(c *gc.C) {
	dummy.SetSupportsSpaceDiscovery(true)
	s.unlockCheck(c, s.assertDiscoveredSpaces)
	s.unlockCheck(c, func(*gc.C) {
		s.assertDiscoveredSpaces(c)
		s.assertNumCalls(c, 2, 2)
	})
}

func (s *WorkerSuite) TestWorkerIgnoresExistingSpacesAndSubnets(c *gc.C) {
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

	s.unlockCheck(c, func(c *gc.C) {
		spaces, err := s.State.AllSpaces()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(spaces, gc.HasLen, 5)
	})
}

func (s *WorkerSuite) startWorker(c *gc.C) (worker.Worker, gate.Lock) {
	// create fresh environ to see any injected broken-ness
	config, err := s.State.ModelConfig()
	c.Assert(err, jc.ErrorIsNil)
	environ, err := environs.New(config)
	c.Assert(err, jc.ErrorIsNil)

	lock := gate.NewLock()
	worker, err := discoverspaces.NewWorker(discoverspaces.Config{
		Facade:   s.API,
		Environ:  environ,
		NewName:  network.ConvertSpaceName,
		Unlocker: lock,
	})
	c.Assert(err, jc.ErrorIsNil)
	return worker, lock
}

func (s *WorkerSuite) unlockCheck(c *gc.C, check func(c *gc.C)) {
	worker, lock := s.startWorker(c)
	defer workertest.CleanKill(c, worker)
	select {
	case <-time.After(coretesting.LongWait):
		c.Fatalf("discovery never completed")
	case <-lock.Unlocked():
		check(c)
	}
	workertest.CheckAlive(c, worker)
}

func (s *WorkerSuite) assertDiscoveredNoSpaces(c *gc.C) {
	spaces, err := s.State.AllSpaces()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(spaces, gc.HasLen, 0)
}

func (s *WorkerSuite) assertDiscoveredSpaces(c *gc.C) {
	spaces, err := s.State.AllSpaces()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(spaces, gc.HasLen, 4)
	expectedSpaces := []network.SpaceInfo{{
		Name:       "foo",
		ProviderId: network.Id("0"),
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
		ProviderId: network.Id("1"),
		Subnets: []network.SubnetInfo{{
			ProviderId:        network.Id("3"),
			CIDR:              "192.168.3.0/24",
			AvailabilityZones: []string{"zone1"},
		}}}, {
		Name:       "foo-2",
		ProviderId: network.Id("2"),
		Subnets: []network.SubnetInfo{{
			ProviderId:        network.Id("4"),
			CIDR:              "192.168.4.0/24",
			AvailabilityZones: []string{"zone1"},
		}}}, {
		Name:       "empty",
		ProviderId: network.Id("3"),
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
}

func (s *WorkerSuite) assertNumCalls(c *gc.C, expectedNumCreateSpaceCalls, expectedNumAddSubnetsCalls int) {
	c.Check(atomic.LoadUint32(&s.numCreateSpaceCalls), gc.Equals, uint32(expectedNumCreateSpaceCalls))
	c.Check(atomic.LoadUint32(&s.numAddSubnetsCalls), gc.Equals, uint32(expectedNumAddSubnetsCalls))
}
