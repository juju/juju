// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	stdcontext "context"
	"fmt"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/envcontext"
	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/provider/common"
	coretesting "github.com/juju/juju/testing"
)

type AvailabilityZoneSuite struct {
	coretesting.FakeJujuXDGDataHomeSuite
	env mockZonedEnviron

	callCtx envcontext.ProviderCallContext
}

var _ = gc.Suite(&AvailabilityZoneSuite{})

func (s *AvailabilityZoneSuite) SetUpSuite(c *gc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpSuite(c)

	s.callCtx = envcontext.WithoutCredentialInvalidator(stdcontext.Background())
	allInstances := make([]instances.Instance, 3)
	for i := range allInstances {
		allInstances[i] = &mockInstance{id: fmt.Sprintf("inst%d", i)}
	}
	s.env.allInstances = func(envcontext.ProviderCallContext) ([]instances.Instance, error) {
		return allInstances, nil
	}

	availabilityZones := make(network.AvailabilityZones, 3)
	for i := range availabilityZones {
		availabilityZones[i] = &mockAvailabilityZone{
			name:      fmt.Sprintf("az%d", i),
			available: i > 0,
		}
	}
	s.env.availabilityZones = func(envcontext.ProviderCallContext) (network.AvailabilityZones, error) {
		return availabilityZones, nil
	}
}

func (s *AvailabilityZoneSuite) TestAvailabilityZoneAllocationsAllRunningInstances(c *gc.C) {
	var called int
	s.PatchValue(&s.env.instanceAvailabilityZoneNames, func(ctx envcontext.ProviderCallContext, ids []instance.Id) (map[instance.Id]string, error) {
		c.Assert(ids, gc.DeepEquals, []instance.Id{"inst0", "inst1", "inst2"})
		called++
		return map[instance.Id]string{
			"inst0": "az0",
			"inst1": "az1",
			"inst2": "az2",
		}, nil
	})
	zoneInstances, err := common.AvailabilityZoneAllocations(&s.env, s.callCtx, nil)
	c.Assert(called, gc.Equals, 1)
	c.Assert(err, jc.ErrorIsNil)
	// az0 is unavailable, so az1 and az2 come out as equal best;
	// az1 comes first due to lexicographical ordering on the name.
	c.Assert(zoneInstances, gc.DeepEquals, []common.AvailabilityZoneInstances{{
		ZoneName:  "az1",
		Instances: []instance.Id{"inst1"},
	}, {
		ZoneName:  "az2",
		Instances: []instance.Id{"inst2"},
	}})
}

func (s *AvailabilityZoneSuite) TestAvailabilityZoneAllocationsAllRunningInstancesErrors(c *gc.C) {
	resultErr := fmt.Errorf("oh noes")
	s.PatchValue(&s.env.allInstances, func(envcontext.ProviderCallContext) ([]instances.Instance, error) {
		return nil, resultErr
	})
	zoneInstances, err := common.AvailabilityZoneAllocations(&s.env, s.callCtx, nil)
	c.Assert(err, gc.Equals, resultErr)
	c.Assert(zoneInstances, gc.HasLen, 0)
}

func (s *AvailabilityZoneSuite) TestAvailabilityZoneAllocationsPartialInstances(c *gc.C) {
	var called int
	s.PatchValue(&s.env.instanceAvailabilityZoneNames, func(ctx envcontext.ProviderCallContext, ids []instance.Id) (map[instance.Id]string, error) {
		c.Assert(ids, gc.DeepEquals, []instance.Id{"nichts", "inst1", "null", "inst2"})
		called++
		return map[instance.Id]string{"inst1": "az1", "inst2": "az1"}, environs.ErrPartialInstances
	})
	zoneInstances, err := common.AvailabilityZoneAllocations(&s.env, s.callCtx, []instance.Id{"nichts", "inst1", "null", "inst2"})
	c.Assert(called, gc.Equals, 1)
	c.Assert(err, jc.ErrorIsNil)
	// az2 has fewer instances, so comes first.
	c.Assert(zoneInstances, gc.DeepEquals, []common.AvailabilityZoneInstances{{
		ZoneName: "az2",
	}, {
		ZoneName:  "az1",
		Instances: []instance.Id{"inst1", "inst2"},
	}})
}

func (s *AvailabilityZoneSuite) TestAvailabilityZoneAllocationsInstanceAvailabilityZonesErrors(c *gc.C) {
	returnErr := fmt.Errorf("whatever")
	var called int
	s.PatchValue(&s.env.instanceAvailabilityZoneNames, func(ctx envcontext.ProviderCallContext, ids []instance.Id) (map[instance.Id]string, error) {
		called++
		return nil, returnErr
	})
	zoneInstances, err := common.AvailabilityZoneAllocations(&s.env, s.callCtx, nil)
	c.Assert(called, gc.Equals, 1)
	c.Assert(err, gc.Equals, returnErr)
	c.Assert(zoneInstances, gc.HasLen, 0)
}

func (s *AvailabilityZoneSuite) TestAvailabilityZoneAllocationsInstanceAvailabilityZonesNoInstances(c *gc.C) {
	var called int
	s.PatchValue(&s.env.instanceAvailabilityZoneNames, func(ctx envcontext.ProviderCallContext, ids []instance.Id) (map[instance.Id]string, error) {
		called++
		return nil, environs.ErrNoInstances
	})
	zoneInstances, err := common.AvailabilityZoneAllocations(&s.env, s.callCtx, nil)
	c.Assert(called, gc.Equals, 1)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(zoneInstances, gc.HasLen, 2)
}

func (s *AvailabilityZoneSuite) TestAvailabilityZoneAllocationsNoZones(c *gc.C) {
	var calls []string
	s.PatchValue(&s.env.instanceAvailabilityZoneNames, func(ctx envcontext.ProviderCallContext, ids []instance.Id) (map[instance.Id]string, error) {
		c.Assert(ids, gc.DeepEquals, []instance.Id{"inst0", "inst1", "inst2"})
		calls = append(calls, "InstanceAvailabilityZoneNames")
		return make(map[instance.Id]string, 3), nil
	})
	s.PatchValue(&s.env.availabilityZones, func(envcontext.ProviderCallContext) (network.AvailabilityZones, error) {
		calls = append(calls, "AvailabilityZones")
		return network.AvailabilityZones{}, nil
	})
	zoneInstances, err := common.AvailabilityZoneAllocations(&s.env, s.callCtx, nil)
	c.Assert(calls, gc.DeepEquals, []string{"InstanceAvailabilityZoneNames", "AvailabilityZones"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(zoneInstances, gc.HasLen, 0)
}

func (s *AvailabilityZoneSuite) TestAvailabilityZoneAllocationsErrors(c *gc.C) {
	var calls []string
	s.PatchValue(&s.env.instanceAvailabilityZoneNames, func(ctx envcontext.ProviderCallContext, ids []instance.Id) (map[instance.Id]string, error) {
		c.Assert(ids, gc.DeepEquals, []instance.Id{"inst0", "inst1", "inst2"})
		calls = append(calls, "InstanceAvailabilityZoneNames")
		return make(map[instance.Id]string, 3), nil
	})
	resultErr := fmt.Errorf("u can haz no az")
	s.PatchValue(&s.env.availabilityZones, func(envcontext.ProviderCallContext) (network.AvailabilityZones, error) {
		calls = append(calls, "AvailabilityZones")
		return nil, resultErr
	})
	zoneInstances, err := common.AvailabilityZoneAllocations(&s.env, s.callCtx, nil)
	c.Assert(calls, gc.DeepEquals, []string{"InstanceAvailabilityZoneNames", "AvailabilityZones"})
	c.Assert(err, gc.Equals, resultErr)
	c.Assert(zoneInstances, gc.HasLen, 0)
}

func (s *AvailabilityZoneSuite) TestDistributeInstancesGroup(c *gc.C) {
	expectedGroup := []instance.Id{"0", "1", "2"}
	var called bool
	s.PatchValue(common.InternalAvailabilityZoneAllocations, func(_ common.ZonedEnviron, ctx envcontext.ProviderCallContext, group []instance.Id) ([]common.AvailabilityZoneInstances, error) {
		c.Assert(group, gc.DeepEquals, expectedGroup)
		called = true
		return nil, nil
	})
	_, err := common.DistributeInstances(&s.env, s.callCtx, nil, expectedGroup, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(called, jc.IsTrue)
}

func (s *AvailabilityZoneSuite) TestDistributeInstancesGroupErrors(c *gc.C) {
	resultErr := fmt.Errorf("whatever")
	s.PatchValue(common.InternalAvailabilityZoneAllocations, func(_ common.ZonedEnviron, ctx envcontext.ProviderCallContext, group []instance.Id) ([]common.AvailabilityZoneInstances, error) {
		return nil, resultErr
	})
	_, err := common.DistributeInstances(&s.env, s.callCtx, nil, nil, nil)
	c.Assert(err, gc.Equals, resultErr)
}

func (s *AvailabilityZoneSuite) TestDistributeInstances(c *gc.C) {
	var zoneInstances []common.AvailabilityZoneInstances
	s.PatchValue(common.InternalAvailabilityZoneAllocations, func(_ common.ZonedEnviron, ctx envcontext.ProviderCallContext, group []instance.Id) ([]common.AvailabilityZoneInstances, error) {
		return zoneInstances, nil
	})

	type distributeInstancesTest struct {
		zoneInstances []common.AvailabilityZoneInstances
		candidates    []instance.Id
		limitZones    []string
		eligible      []instance.Id
	}

	defaultZoneInstances := []common.AvailabilityZoneInstances{{
		ZoneName:  "az0",
		Instances: []instance.Id{"i0"},
	}, {
		ZoneName:  "az1",
		Instances: []instance.Id{"i1"},
	}, {
		ZoneName:  "az2",
		Instances: []instance.Id{"i2"},
	}}

	tests := []distributeInstancesTest{{
		zoneInstances: defaultZoneInstances,
		candidates:    []instance.Id{"i2", "i3", "i4"},
		eligible:      []instance.Id{"i2"},
	}, {
		zoneInstances: defaultZoneInstances,
		candidates:    []instance.Id{"i0", "i1", "i2"},
		eligible:      []instance.Id{"i0", "i1", "i2"},
	}, {
		zoneInstances: defaultZoneInstances,
		candidates:    []instance.Id{"i3", "i4", "i5"},
		eligible:      []instance.Id{},
	}, {
		zoneInstances: defaultZoneInstances,
		candidates:    []instance.Id{},
		eligible:      []instance.Id{},
	}, {
		zoneInstances: []common.AvailabilityZoneInstances{},
		candidates:    []instance.Id{"i0"},
		eligible:      []instance.Id{},
	}, {
		// Limit to all zones; essentially the same as no limit.
		zoneInstances: defaultZoneInstances,
		candidates:    []instance.Id{"i0", "i1", "i2"},
		limitZones:    []string{"az0", "az1", "az2"},
		eligible:      []instance.Id{"i0", "i1", "i2"},
	}, {
		// Simple limit to a subset of zones.
		zoneInstances: defaultZoneInstances,
		candidates:    []instance.Id{"i0", "i1", "i2"},
		limitZones:    []string{"az0", "az1"},
		eligible:      []instance.Id{"i0", "i1"},
	}, {
		// Intersecting zone limit with equal distribution.
		zoneInstances: defaultZoneInstances,
		candidates:    []instance.Id{"i0", "i1"},
		limitZones:    []string{"az1", "az2", "az4"},
		eligible:      []instance.Id{"i0", "i1"},
	}, {
		// Intersecting zone limit with unequal distribution.
		zoneInstances: []common.AvailabilityZoneInstances{{
			ZoneName:  "az0",
			Instances: []instance.Id{"i0"},
		}, {
			ZoneName:  "az1",
			Instances: []instance.Id{"i1", "i2"},
		}},
		candidates: []instance.Id{"i0", "i1", "i2"},
		limitZones: []string{"az0", "az1", "az666"},
		eligible:   []instance.Id{"i0"},
	}, {
		// Limit filters out all zones - no eligible instances.
		zoneInstances: []common.AvailabilityZoneInstances{{
			ZoneName:  "az0",
			Instances: []instance.Id{"i0"},
		}, {
			ZoneName:  "az1",
			Instances: []instance.Id{"i1"},
		}},
		candidates: []instance.Id{"i0", "i1"},
		limitZones: []string{"az2", "az3"},
		eligible:   []instance.Id{},
	}}

	for i, test := range tests {
		c.Logf("test %d", i)
		zoneInstances = test.zoneInstances
		eligible, err := common.DistributeInstances(&s.env, s.callCtx, test.candidates, nil, test.limitZones)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(eligible, jc.SameContents, test.eligible)
	}
}
