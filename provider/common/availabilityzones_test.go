// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"fmt"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/provider/common"
	coretesting "github.com/juju/juju/testing"
)

type AvailabilityZoneSuite struct {
	coretesting.FakeJujuXDGDataHomeSuite
	env mockZonedEnviron

	callCtx context.ProviderCallContext
}

var _ = gc.Suite(&AvailabilityZoneSuite{})

func (s *AvailabilityZoneSuite) SetUpSuite(c *gc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpSuite(c)

	s.callCtx = context.NewCloudCallContext()
	allInstances := make([]instance.Instance, 3)
	for i := range allInstances {
		allInstances[i] = &mockInstance{id: fmt.Sprintf("inst%d", i)}
	}
	s.env.allInstances = func(context.ProviderCallContext) ([]instance.Instance, error) {
		return allInstances, nil
	}

	availabilityZones := make([]common.AvailabilityZone, 3)
	for i := range availabilityZones {
		availabilityZones[i] = &mockAvailabilityZone{
			name:      fmt.Sprintf("az%d", i),
			available: i > 0,
		}
	}
	s.env.availabilityZones = func(context.ProviderCallContext) ([]common.AvailabilityZone, error) {
		return availabilityZones, nil
	}
}

func (s *AvailabilityZoneSuite) TestAvailabilityZoneAllocationsAllInstances(c *gc.C) {
	var called int
	s.PatchValue(&s.env.instanceAvailabilityZoneNames, func(ctx context.ProviderCallContext, ids []instance.Id) ([]string, error) {
		c.Assert(ids, gc.DeepEquals, []instance.Id{"inst0", "inst1", "inst2"})
		called++
		return []string{"az0", "az1", "az2"}, nil
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

func (s *AvailabilityZoneSuite) TestAvailabilityZoneAllocationsAllInstancesErrors(c *gc.C) {
	resultErr := fmt.Errorf("oh noes")
	s.PatchValue(&s.env.allInstances, func(context.ProviderCallContext) ([]instance.Instance, error) {
		return nil, resultErr
	})
	zoneInstances, err := common.AvailabilityZoneAllocations(&s.env, s.callCtx, nil)
	c.Assert(err, gc.Equals, resultErr)
	c.Assert(zoneInstances, gc.HasLen, 0)
}

func (s *AvailabilityZoneSuite) TestAvailabilityZoneAllocationsPartialInstances(c *gc.C) {
	var called int
	s.PatchValue(&s.env.instanceAvailabilityZoneNames, func(ctx context.ProviderCallContext, ids []instance.Id) ([]string, error) {
		c.Assert(ids, gc.DeepEquals, []instance.Id{"nichts", "inst1", "null", "inst2"})
		called++
		return []string{"", "az1", "", "az1"}, environs.ErrPartialInstances
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
	s.PatchValue(&s.env.instanceAvailabilityZoneNames, func(ctx context.ProviderCallContext, ids []instance.Id) ([]string, error) {
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
	s.PatchValue(&s.env.instanceAvailabilityZoneNames, func(ctx context.ProviderCallContext, ids []instance.Id) ([]string, error) {
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
	s.PatchValue(&s.env.instanceAvailabilityZoneNames, func(ctx context.ProviderCallContext, ids []instance.Id) ([]string, error) {
		c.Assert(ids, gc.DeepEquals, []instance.Id{"inst0", "inst1", "inst2"})
		calls = append(calls, "InstanceAvailabilityZoneNames")
		return []string{"", "", ""}, nil
	})
	s.PatchValue(&s.env.availabilityZones, func(context.ProviderCallContext) ([]common.AvailabilityZone, error) {
		calls = append(calls, "AvailabilityZones")
		return []common.AvailabilityZone{}, nil
	})
	zoneInstances, err := common.AvailabilityZoneAllocations(&s.env, s.callCtx, nil)
	c.Assert(calls, gc.DeepEquals, []string{"InstanceAvailabilityZoneNames", "AvailabilityZones"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(zoneInstances, gc.HasLen, 0)
}

func (s *AvailabilityZoneSuite) TestAvailabilityZoneAllocationsErrors(c *gc.C) {
	var calls []string
	s.PatchValue(&s.env.instanceAvailabilityZoneNames, func(ctx context.ProviderCallContext, ids []instance.Id) ([]string, error) {
		c.Assert(ids, gc.DeepEquals, []instance.Id{"inst0", "inst1", "inst2"})
		calls = append(calls, "InstanceAvailabilityZoneNames")
		return []string{"", "", ""}, nil
	})
	resultErr := fmt.Errorf("u can haz no az")
	s.PatchValue(&s.env.availabilityZones, func(context.ProviderCallContext) ([]common.AvailabilityZone, error) {
		calls = append(calls, "AvailabilityZones")
		return nil, resultErr
	})
	zoneInstances, err := common.AvailabilityZoneAllocations(&s.env, s.callCtx, nil)
	c.Assert(calls, gc.DeepEquals, []string{"InstanceAvailabilityZoneNames", "AvailabilityZones"})
	c.Assert(err, gc.Equals, resultErr)
	c.Assert(zoneInstances, gc.HasLen, 0)
}

func (s *AvailabilityZoneSuite) TestValidateAvailabilityZone(c *gc.C) {
	var calls []string
	s.PatchValue(&s.env.availabilityZones, func(context.ProviderCallContext) ([]common.AvailabilityZone, error) {
		availabilityZones := make([]common.AvailabilityZone, 2)
		availabilityZones[0] = &mockAvailabilityZone{name: "az1", available: true}
		availabilityZones[1] = &mockAvailabilityZone{name: "az2", available: false}
		calls = append(calls, "AvailabilityZones")
		return availabilityZones, nil
	})
	tests := map[string]error{
		"az1": nil,
		"az2": errors.Errorf("availability zone %q is unavailable", "az2"),
		"az3": errors.NotValidf("availability zone %q", "az3"),
	}
	for i, t := range tests {
		err := common.ValidateAvailabilityZone(&s.env, s.callCtx, i)
		if t == nil {
			c.Assert(err, jc.ErrorIsNil)
		} else {
			c.Assert(err, gc.ErrorMatches, err.Error())
		}
		c.Assert(calls, gc.DeepEquals, []string{"AvailabilityZones"})
		calls = []string{}
	}
}

func (s *AvailabilityZoneSuite) TestDistributeInstancesGroup(c *gc.C) {
	expectedGroup := []instance.Id{"0", "1", "2"}
	var called bool
	s.PatchValue(common.InternalAvailabilityZoneAllocations, func(_ common.ZonedEnviron, ctx context.ProviderCallContext, group []instance.Id) ([]common.AvailabilityZoneInstances, error) {
		c.Assert(group, gc.DeepEquals, expectedGroup)
		called = true
		return nil, nil
	})
	common.DistributeInstances(&s.env, s.callCtx, nil, expectedGroup)
	c.Assert(called, jc.IsTrue)
}

func (s *AvailabilityZoneSuite) TestDistributeInstancesGroupErrors(c *gc.C) {
	resultErr := fmt.Errorf("whatever")
	s.PatchValue(common.InternalAvailabilityZoneAllocations, func(_ common.ZonedEnviron, ctx context.ProviderCallContext, group []instance.Id) ([]common.AvailabilityZoneInstances, error) {
		return nil, resultErr
	})
	_, err := common.DistributeInstances(&s.env, s.callCtx, nil, nil)
	c.Assert(err, gc.Equals, resultErr)
}

func (s *AvailabilityZoneSuite) TestDistributeInstances(c *gc.C) {
	var zoneInstances []common.AvailabilityZoneInstances
	s.PatchValue(common.InternalAvailabilityZoneAllocations, func(_ common.ZonedEnviron, ctx context.ProviderCallContext, group []instance.Id) ([]common.AvailabilityZoneInstances, error) {
		return zoneInstances, nil
	})

	type distributeInstancesTest struct {
		zoneInstances []common.AvailabilityZoneInstances
		candidates    []instance.Id
		eligible      []instance.Id
	}

	tests := []distributeInstancesTest{{
		zoneInstances: []common.AvailabilityZoneInstances{{
			ZoneName:  "az0",
			Instances: []instance.Id{"i0"},
		}, {
			ZoneName:  "az1",
			Instances: []instance.Id{"i1"},
		}, {
			ZoneName:  "az2",
			Instances: []instance.Id{"i2"},
		}},
		candidates: []instance.Id{"i2", "i3", "i4"},
		eligible:   []instance.Id{"i2"},
	}, {
		zoneInstances: []common.AvailabilityZoneInstances{{
			ZoneName:  "az0",
			Instances: []instance.Id{"i0"},
		}, {
			ZoneName:  "az1",
			Instances: []instance.Id{"i1"},
		}, {
			ZoneName:  "az2",
			Instances: []instance.Id{"i2"},
		}},
		candidates: []instance.Id{"i0", "i1", "i2"},
		eligible:   []instance.Id{"i0", "i1", "i2"},
	}, {
		zoneInstances: []common.AvailabilityZoneInstances{{
			ZoneName:  "az0",
			Instances: []instance.Id{"i0"},
		}, {
			ZoneName:  "az1",
			Instances: []instance.Id{"i1"},
		}, {
			ZoneName:  "az2",
			Instances: []instance.Id{"i2"},
		}},
		candidates: []instance.Id{"i3", "i4", "i5"},
		eligible:   []instance.Id{},
	}, {
		zoneInstances: []common.AvailabilityZoneInstances{{
			ZoneName:  "az0",
			Instances: []instance.Id{"i0"},
		}, {
			ZoneName:  "az1",
			Instances: []instance.Id{"i1"},
		}, {
			ZoneName:  "az2",
			Instances: []instance.Id{"i2"},
		}},
		candidates: []instance.Id{},
		eligible:   []instance.Id{},
	}, {
		zoneInstances: []common.AvailabilityZoneInstances{},
		candidates:    []instance.Id{"i0"},
		eligible:      []instance.Id{},
	}}

	for i, test := range tests {
		c.Logf("test %d", i)
		zoneInstances = test.zoneInstances
		eligible, err := common.DistributeInstances(&s.env, s.callCtx, test.candidates, nil)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(eligible, jc.SameContents, test.eligible)
	}
}
