// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package discoverspaces_test

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	worker "gopkg.in/juju/worker.v1"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/discoverspaces"
	"github.com/juju/juju/worker/gate"
	"github.com/juju/juju/worker/workertest"
)

type WorkerSuite struct {
	coretesting.BaseSuite
	facade          fakeFacade
	environ         fakeEnviron
	selectedEnviron environs.Environ
}

var _ = gc.Suite(&WorkerSuite{})

func (s *WorkerSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.facade = fakeFacade{stub: &testing.Stub{}}
	s.environ = fakeEnviron{
		stub:           &testing.Stub{},
		spaceDiscovery: true,
	}
	s.selectedEnviron = &s.environ
}

func (s *WorkerSuite) TestSupportsSpaceDiscoveryBroken(c *gc.C) {
	s.environ.stub.SetErrors(errors.New("SupportsSpaceDiscovery is broken"))

	worker, lock := s.startWorker(c)
	err := workertest.CheckKilled(c, worker)
	c.Assert(err, gc.ErrorMatches, "SupportsSpaceDiscovery is broken")

	select {
	case <-time.After(coretesting.ShortWait):
	case <-lock.Unlocked():
		c.Fatalf("gate unlocked despite worker failure")
	}
}

func (s *WorkerSuite) TestSpacesBroken(c *gc.C) {
	s.environ.stub.SetErrors(nil, errors.New("Spaces am broken"))
	worker, lock := s.startWorker(c)
	err := workertest.CheckKilled(c, worker)
	c.Assert(err, gc.ErrorMatches, "Spaces am broken")

	select {
	case <-time.After(coretesting.ShortWait):
	case <-lock.Unlocked():
		c.Fatalf("gate unlocked despite worker failure")
	}
}

func (s *WorkerSuite) TestWorkerSupportsNetworkingFalse(c *gc.C) {
	s.selectedEnviron = &fakeNoNetworkEnviron{}
	s.unlockCheck(c, s.assertDiscoveredNoSpaces)
}

func (s *WorkerSuite) cannedSubnets() []network.SubnetInfo {
	return []network.SubnetInfo{{
		ProviderId:        network.Id("1"),
		CIDR:              "192.168.1.0/24",
		AvailabilityZones: []string{"zone1"},
	}, {
		ProviderId:        network.Id("2"),
		CIDR:              "192.168.2.0/24",
		AvailabilityZones: []string{"zone1"},
	}, {
		ProviderId:        network.Id("3"),
		CIDR:              "192.168.3.0/24",
		AvailabilityZones: []string{"zone1"},
	}, {
		ProviderId:        network.Id("4"),
		CIDR:              "192.168.4.0/24",
		AvailabilityZones: []string{"zone1"},
	}, {
		ProviderId:        network.Id("5"),
		CIDR:              "192.168.5.0/24",
		AvailabilityZones: []string{"zone1"},
	}}
}

func (s *WorkerSuite) TestWorkerNoSpaceDiscoveryOnlySubnets(c *gc.C) {
	s.environ.spaceDiscovery = false
	s.environ.subnets = s.cannedSubnets()
	s.facade.addSubnets = makeErrorResults(nil, nil, nil, nil, nil)
	s.unlockCheck(c, func(c *gc.C) {
		stub := s.facade.stub
		stub.CheckCallNames(c, "ListSubnets", "AddSubnets")
		stub.CheckCall(c, 1, "AddSubnets", params.AddSubnetsParams{
			Subnets: []params.AddSubnetParams{{
				SubnetProviderId: "1",
				Zones:            []string{"zone1"},
			}, {
				SubnetProviderId: "2",
				Zones:            []string{"zone1"},
			}, {
				SubnetProviderId: "3",
				Zones:            []string{"zone1"},
			}, {
				SubnetProviderId: "4",
				Zones:            []string{"zone1"},
			}, {
				SubnetProviderId: "5",
				Zones:            []string{"zone1"},
			}},
		})
		s.environ.stub.CheckCallNames(c, "SupportsSpaceDiscovery", "Subnets")
		s.environ.stub.CheckCall(c, 1, "Subnets", instance.UnknownId, []network.Id{})
	})
}

func (s *WorkerSuite) cannedSpaces() []network.SpaceInfo {
	return []network.SpaceInfo{{
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
		}},
	}, {
		Name:       "Another foo 99!",
		ProviderId: network.Id("1"),
		Subnets: []network.SubnetInfo{{
			ProviderId:        network.Id("3"),
			CIDR:              "192.168.3.0/24",
			AvailabilityZones: []string{"zone1"},
		}},
	}, {
		Name:       "foo-",
		ProviderId: network.Id("2"),
		Subnets: []network.SubnetInfo{{
			ProviderId:        network.Id("4"),
			CIDR:              "192.168.4.0/24",
			AvailabilityZones: []string{"zone1"},
		}},
	}, {
		Name:       "---",
		ProviderId: network.Id("3"),
		Subnets: []network.SubnetInfo{{
			ProviderId:        network.Id("5"),
			CIDR:              "192.168.5.0/24",
			AvailabilityZones: []string{"zone1"},
		}},
	}}
}

func makeErrorResults(errors ...*params.Error) params.ErrorResults {
	results := make([]params.ErrorResult, len(errors))
	for i, err := range errors {
		results[i].Error = err
	}
	return params.ErrorResults{Results: results}
}

func (s *WorkerSuite) TestWorkerDiscoversSpaces(c *gc.C) {
	s.environ.spaces = s.cannedSpaces()
	s.facade.createSpaces = makeErrorResults(nil, nil, nil, nil)
	s.facade.addSubnets = makeErrorResults(nil, nil, nil, nil, nil)
	// The facade reports there aren't any spaces or subnets already.
	s.unlockCheck(c, func(*gc.C) {
		stub := s.facade.stub
		stub.CheckCallNames(c, "ListSpaces", "ListSubnets", "CreateSpaces", "AddSubnets")
		stub.CheckCall(c, 2, "CreateSpaces", params.CreateSpacesParams{
			Spaces: []params.CreateSpaceParams{{
				Public:     false,
				SpaceTag:   "space-foo",
				ProviderId: "0",
			}, {
				Public:     false,
				SpaceTag:   "space-another-foo-99",
				ProviderId: "1",
			}, {
				Public:     false,
				SpaceTag:   "space-foo-2",
				ProviderId: "2",
			}, {
				Public:     false,
				SpaceTag:   "space-empty",
				ProviderId: "3",
			}},
		})
		stub.CheckCall(c, 3, "AddSubnets", params.AddSubnetsParams{
			Subnets: []params.AddSubnetParams{{
				SubnetProviderId: "1",
				SpaceTag:         "space-foo",
				Zones:            []string{"zone1"},
			}, {
				SubnetProviderId: "2",
				SpaceTag:         "space-foo",
				Zones:            []string{"zone1"},
			}, {
				SubnetProviderId: "3",
				SpaceTag:         "space-another-foo-99",
				Zones:            []string{"zone1"},
			}, {
				SubnetProviderId: "4",
				SpaceTag:         "space-foo-2",
				Zones:            []string{"zone1"},
			}, {
				SubnetProviderId: "5",
				SpaceTag:         "space-empty",
				Zones:            []string{"zone1"},
			}},
		})
	})
}

func (s *WorkerSuite) TestWorkerIgnoresExistingSpacesAndSubnets(c *gc.C) {
	s.environ.spaces = s.cannedSpaces()
	// Indicate that we already know about some of those spaces and
	// subnets.
	s.facade.listSpaces = params.DiscoverSpacesResults{Results: []params.ProviderSpace{
		{ProviderId: "0", Name: "foo"},
		{ProviderId: "1"},
		{ProviderId: "3"},
	}}
	s.facade.listSubnets = params.ListSubnetsResults{Results: []params.Subnet{
		{ProviderId: "1"},
		{ProviderId: "3"},
		{ProviderId: "5"},
	}}
	s.facade.createSpaces = makeErrorResults(nil)
	s.facade.addSubnets = makeErrorResults(nil, nil)
	s.unlockCheck(c, func(*gc.C) {
		stub := s.facade.stub
		// We only create the new ones.
		stub.CheckCallNames(c, "ListSpaces", "ListSubnets", "CreateSpaces", "AddSubnets")
		stub.CheckCall(c, 2, "CreateSpaces", params.CreateSpacesParams{
			Spaces: []params.CreateSpaceParams{{
				Public:     false,
				SpaceTag:   "space-foo-2",
				ProviderId: "2",
			}},
		})
		stub.CheckCall(c, 3, "AddSubnets", params.AddSubnetsParams{
			Subnets: []params.AddSubnetParams{{
				SubnetProviderId: "2",
				SpaceTag:         "space-foo",
				Zones:            []string{"zone1"},
			}, {
				SubnetProviderId: "4",
				SpaceTag:         "space-foo-2",
				Zones:            []string{"zone1"},
			}},
		})
	})
}

func (s *WorkerSuite) TestWorkerIdempotent(c *gc.C) {
	s.environ.spaces = s.cannedSpaces()
	// Indicate that we already know about all of those spaces and
	// subnets.
	s.facade.listSpaces = params.DiscoverSpacesResults{Results: []params.ProviderSpace{
		{ProviderId: "0"},
		{ProviderId: "1"},
		{ProviderId: "2"},
		{ProviderId: "3"},
	}}
	s.facade.listSubnets = params.ListSubnetsResults{Results: []params.Subnet{
		{ProviderId: "1"},
		{ProviderId: "2"},
		{ProviderId: "3"},
		{ProviderId: "4"},
		{ProviderId: "5"},
	}}
	s.unlockCheck(c, func(*gc.C) {
		stub := s.facade.stub
		// We don't create any more.
		stub.CheckCallNames(c, "ListSpaces", "ListSubnets")
	})
}

func (s *WorkerSuite) startWorker(c *gc.C) (worker.Worker, gate.Lock) {
	lock := gate.NewLock()
	worker, err := discoverspaces.NewWorker(discoverspaces.Config{
		Facade:   &s.facade,
		Environ:  s.selectedEnviron,
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
	for _, call := range s.facade.stub.Calls() {
		if call.FuncName == "CreateSpaces" {
			c.Fatalf("created some spaces: %#v", call.Args)
		}
	}
}
