// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"fmt"

	jc "github.com/juju/testing/checkers"
	gc "launchpad.net/gocheck"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/provider/common"
	coretesting "github.com/juju/juju/testing"
)

type AvailabilityZoneSuite struct {
	coretesting.FakeJujuHomeSuite
	env mockZonedEnviron
}

var _ = gc.Suite(&AvailabilityZoneSuite{})

func (s *AvailabilityZoneSuite) SetUpSuite(c *gc.C) {
	s.FakeJujuHomeSuite.SetUpSuite(c)

	allInstances := make([]instance.Instance, 3)
	for i := range allInstances {
		allInstances[i] = &mockInstance{id: fmt.Sprintf("inst%d", i)}
	}
	s.env.allInstances = func() ([]instance.Instance, error) {
		return allInstances, nil
	}

	availabilityZones := make([]common.AvailabilityZone, 3)
	for i := range availabilityZones {
		availabilityZones[i] = &mockAvailabilityZone{
			name:      fmt.Sprintf("az%d", i),
			available: i > 0,
		}
	}
	s.env.availabilityZones = func() ([]common.AvailabilityZone, error) {
		return availabilityZones, nil
	}
}

func (s *AvailabilityZoneSuite) TestBestAvailabilityZoneAllocationsAllInstances(c *gc.C) {
	var called int
	s.PatchValue(&s.env.instanceAvailabilityZoneNames, func(ids []instance.Id) ([]string, error) {
		c.Assert(ids, gc.DeepEquals, []instance.Id{"inst0", "inst1", "inst2"})
		called++
		return []string{"az0", "az1", "az2"}, nil
	})
	best, err := common.BestAvailabilityZoneAllocations(&s.env, nil)
	c.Assert(called, gc.Equals, 1)
	c.Assert(err, gc.IsNil)
	// az0 is unavailable, so az1 and az2 come out as equal best.
	c.Assert(best, gc.DeepEquals, map[string][]instance.Id{
		"az1": []instance.Id{"inst1"},
		"az2": []instance.Id{"inst2"},
	})
}

func (s *AvailabilityZoneSuite) TestBestAvailabilityZoneAllocationsAllInstancesErrors(c *gc.C) {
	resultErr := fmt.Errorf("oh noes")
	s.PatchValue(&s.env.allInstances, func() ([]instance.Instance, error) {
		return nil, resultErr
	})
	best, err := common.BestAvailabilityZoneAllocations(&s.env, nil)
	c.Assert(err, gc.Equals, resultErr)
	c.Assert(best, gc.HasLen, 0)
}

func (s *AvailabilityZoneSuite) TestBestAvailabilityZoneAllocationsPartialInstances(c *gc.C) {
	var called int
	s.PatchValue(&s.env.instanceAvailabilityZoneNames, func(ids []instance.Id) ([]string, error) {
		c.Assert(ids, gc.DeepEquals, []instance.Id{"nichts", "inst1", "null", "inst2"})
		called++
		return []string{"", "az1", "", "az1"}, environs.ErrPartialInstances
	})
	best, err := common.BestAvailabilityZoneAllocations(&s.env, []instance.Id{"nichts", "inst1", "null", "inst2"})
	c.Assert(called, gc.Equals, 1)
	c.Assert(err, gc.IsNil)
	// All known instances are in az1 and az0 is unavailable, so az2 is the best.
	c.Assert(best, gc.DeepEquals, map[string][]instance.Id{"az2": nil})
}

func (s *AvailabilityZoneSuite) TestBestAvailabilityZoneAllocationsInstanceAvailabilityZonesErrors(c *gc.C) {
	var returnErr error
	var called int
	s.PatchValue(&s.env.instanceAvailabilityZoneNames, func(ids []instance.Id) ([]string, error) {
		called++
		return nil, returnErr
	})
	errors := []error{environs.ErrNoInstances, fmt.Errorf("whatever")}
	for i, err := range errors {
		returnErr = err
		best, err := common.BestAvailabilityZoneAllocations(&s.env, nil)
		c.Assert(called, gc.Equals, i+1)
		c.Assert(err, gc.Equals, returnErr)
		c.Assert(best, gc.HasLen, 0)
	}
}

func (s *AvailabilityZoneSuite) TestBestAvailabilityZoneAllocationsNoZones(c *gc.C) {
	var calls []string
	s.PatchValue(&s.env.instanceAvailabilityZoneNames, func(ids []instance.Id) ([]string, error) {
		c.Assert(ids, gc.DeepEquals, []instance.Id{"inst0", "inst1", "inst2"})
		calls = append(calls, "InstanceAvailabilityZoneNames")
		return []string{"", "", ""}, nil
	})
	s.PatchValue(&s.env.availabilityZones, func() ([]common.AvailabilityZone, error) {
		calls = append(calls, "AvailabilityZones")
		return []common.AvailabilityZone{}, nil
	})
	best, err := common.BestAvailabilityZoneAllocations(&s.env, nil)
	c.Assert(calls, gc.DeepEquals, []string{"InstanceAvailabilityZoneNames", "AvailabilityZones"})
	c.Assert(err, gc.IsNil)
	c.Assert(best, gc.HasLen, 0)
}

func (s *AvailabilityZoneSuite) TestBestAvailabilityZoneAllocationsErrors(c *gc.C) {
	var calls []string
	s.PatchValue(&s.env.instanceAvailabilityZoneNames, func(ids []instance.Id) ([]string, error) {
		c.Assert(ids, gc.DeepEquals, []instance.Id{"inst0", "inst1", "inst2"})
		calls = append(calls, "InstanceAvailabilityZoneNames")
		return []string{"", "", ""}, nil
	})
	resultErr := fmt.Errorf("u can haz no az")
	s.PatchValue(&s.env.availabilityZones, func() ([]common.AvailabilityZone, error) {
		calls = append(calls, "AvailabilityZones")
		return nil, resultErr
	})
	best, err := common.BestAvailabilityZoneAllocations(&s.env, nil)
	c.Assert(calls, gc.DeepEquals, []string{"InstanceAvailabilityZoneNames", "AvailabilityZones"})
	c.Assert(err, gc.Equals, resultErr)
	c.Assert(best, gc.HasLen, 0)
}

func (s *AvailabilityZoneSuite) TestDistributeInstancesGroup(c *gc.C) {
	expectedGroup := []instance.Id{"0", "1", "2"}
	var called bool
	s.PatchValue(common.InternalBestAvailabilityZoneAllocations, func(_ common.ZonedEnviron, group []instance.Id) (map[string][]instance.Id, error) {
		c.Assert(group, gc.DeepEquals, expectedGroup)
		called = true
		return nil, nil
	})
	common.DistributeInstances(&s.env, nil, expectedGroup)
	c.Assert(called, jc.IsTrue)
}

func (s *AvailabilityZoneSuite) TestDistributeInstancesGroupErrors(c *gc.C) {
	resultErr := fmt.Errorf("whatever")
	s.PatchValue(common.InternalBestAvailabilityZoneAllocations, func(_ common.ZonedEnviron, group []instance.Id) (map[string][]instance.Id, error) {
		return nil, resultErr
	})
	_, err := common.DistributeInstances(&s.env, nil, nil)
	c.Assert(err, gc.Equals, resultErr)
}

func (s *AvailabilityZoneSuite) TestDistributeInstances(c *gc.C) {
	var bestAvailabilityZones map[string][]instance.Id
	s.PatchValue(common.InternalBestAvailabilityZoneAllocations, func(_ common.ZonedEnviron, group []instance.Id) (map[string][]instance.Id, error) {
		return bestAvailabilityZones, nil
	})

	type distributeInstancesTest struct {
		bestAvailabilityZones map[string][]instance.Id
		candidates            []instance.Id
		eligible              []instance.Id
	}

	tests := []distributeInstancesTest{{
		bestAvailabilityZones: map[string][]instance.Id{
			"az0": []instance.Id{"i0"},
			"az1": []instance.Id{"i1"},
			"az2": []instance.Id{"i2"},
		},
		candidates: []instance.Id{"i2", "i3", "i4"},
		eligible:   []instance.Id{"i2"},
	}, {
		bestAvailabilityZones: map[string][]instance.Id{
			"az0": []instance.Id{"i0"},
			"az1": []instance.Id{"i1"},
			"az2": []instance.Id{"i2"},
		},
		candidates: []instance.Id{"i0", "i1", "i2"},
		eligible:   []instance.Id{"i0", "i1", "i2"},
	}, {
		bestAvailabilityZones: map[string][]instance.Id{
			"az0": []instance.Id{"i0"},
			"az1": []instance.Id{"i1"},
			"az2": []instance.Id{"i2"},
		},
		candidates: []instance.Id{"i3", "i4", "i5"},
		eligible:   []instance.Id{},
	}, {
		bestAvailabilityZones: map[string][]instance.Id{
			"az0": []instance.Id{"i0"},
			"az1": []instance.Id{"i1"},
			"az2": []instance.Id{"i2"},
		},
		candidates: []instance.Id{},
		eligible:   []instance.Id{},
	}, {
		bestAvailabilityZones: map[string][]instance.Id{},
		candidates:            []instance.Id{"i0"},
		eligible:              []instance.Id{},
	}}

	for i, test := range tests {
		c.Logf("test %d", i)
		bestAvailabilityZones = test.bestAvailabilityZones
		eligible, err := common.DistributeInstances(&s.env, test.candidates, nil)
		c.Assert(err, gc.IsNil)
		c.Assert(eligible, jc.SameContents, test.eligible)
	}
}
