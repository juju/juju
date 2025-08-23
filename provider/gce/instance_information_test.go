// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce

import (
	"time"

	"cloud.google.com/go/compute/apiv1/computepb"
	"github.com/juju/clock/testclock"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/constraints"
)

type instanceInformationSuite struct {
	BaseSuite
}

var _ = gc.Suite(&instanceInformationSuite{})

func (s *instanceInformationSuite) TestInstanceTypesCacheExpiration(c *gc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	env := s.SetupEnv(c, s.MockService)

	s.MockService.EXPECT().AvailabilityZones(gomock.Any(), "us-east1").
		Return([]*computepb.Zone{{Name: ptr("us-east1")}}, nil).Times(3)

	now := time.Now()
	clk := testclock.NewClock(now)
	allInstTypes, err := env.getAllInstanceTypes(s.CallCtx, clk)
	c.Assert(err, jc.ErrorIsNil)

	// Cache miss
	cacheExpAt := env.instCacheExpireAt
	c.Assert(cacheExpAt.After(now), jc.IsTrue, gc.Commentf("expected a cache expiration time to be set"))

	// Cache hit
	cachedInstTypes, err := env.getAllInstanceTypes(s.CallCtx, clk)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(allInstTypes, gc.DeepEquals, cachedInstTypes, gc.Commentf("expected to get cached instance list"))
	c.Assert(env.instCacheExpireAt, gc.Equals, cacheExpAt, gc.Commentf("expected cache expiration timestamp not to be modified"))

	// Forced cache-miss after expiry.
	// NOTE(achilleasa): this will trigger a "advancing a clock with nothing waiting"
	// warning but that's a false positive; we just want to advance the clock
	// to test the cache expiry logic.
	clk.Advance(11 * time.Minute)
	_, err = env.getAllInstanceTypes(s.CallCtx, clk)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(env.instCacheExpireAt.After(cacheExpAt), jc.IsTrue, gc.Commentf("expected cache expiration to be updated"))
	c.Assert(env.instCacheExpireAt.After(clk.Now()), jc.IsTrue, gc.Commentf("expected cache expiration to be in the future"))
}

func (s *instanceInformationSuite) TestEnsureDefaultConstraints(c *gc.C) {
	// Fill default cores and mem.
	cons := constraints.Value{}
	c.Assert(cons.String(), gc.Equals, ``)
	cons = ensureDefaultConstraints(cons)
	c.Assert(cons.String(), gc.Equals, `cores=2`)

	var err error
	// Do not fill default cores and mem if instance type is provided.
	cons, err = constraints.Parse(`instance-type=e2-medium`)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cons.String(), gc.Equals, `instance-type=e2-medium`)
	cons = ensureDefaultConstraints(cons)
	c.Assert(cons.String(), gc.Equals, `instance-type=e2-medium`)

	// Do not fill default cores and mem if cores or/and mem are provided.
	cons, err = constraints.Parse(`cores=1 mem=1024M`) // smaller than defaults
	c.Assert(err, jc.ErrorIsNil)
	cons = ensureDefaultConstraints(cons)
	c.Assert(cons.String(), gc.Equals, `cores=1 mem=1024M`)

	cons, err = constraints.Parse(`cores=4 mem=4096M`)
	c.Assert(err, jc.ErrorIsNil)
	cons = ensureDefaultConstraints(cons)
	c.Assert(cons.String(), gc.Equals, `cores=4 mem=4096M`)

	cons, err = constraints.Parse(`cores=4`)
	c.Assert(err, jc.ErrorIsNil)
	cons = ensureDefaultConstraints(cons)
	c.Assert(cons.String(), gc.Equals, `cores=4`)

	cons, err = constraints.Parse(`mem=4096M`)
	c.Assert(err, jc.ErrorIsNil)
	cons = ensureDefaultConstraints(cons)
	c.Assert(cons.String(), gc.Equals, `cores=2 mem=4096M`)
}
