// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce

import (
	"time"

	"github.com/juju/clock/testclock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/provider/gce/google"
	jc "github.com/juju/testing/checkers"
)

type instanceInformationSuite struct {
	BaseSuite
}

var _ = gc.Suite(&instanceInformationSuite{})

func (s *instanceInformationSuite) TestInstanceTypesCacheExpiration(c *gc.C) {
	zone := google.NewZone("a-zone", google.StatusUp, "", "")
	s.FakeConn.Zones = []google.AvailabilityZone{zone}

	now := time.Now()
	clk := testclock.NewClock(now)
	allInstTypes, err := s.Env.getAllInstanceTypes(s.CallCtx, clk)
	c.Assert(err, jc.ErrorIsNil)

	// Cache miss
	cacheExpAt := s.Env.instCacheExpireAt
	c.Assert(cacheExpAt.After(now), jc.IsTrue, gc.Commentf("expected a cache expiration time to be set"))

	// Cache hit
	cachedInstTypes, err := s.Env.getAllInstanceTypes(s.CallCtx, clk)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(allInstTypes, gc.DeepEquals, cachedInstTypes, gc.Commentf("expected to get cached instance list"))
	c.Assert(s.Env.instCacheExpireAt, gc.Equals, cacheExpAt, gc.Commentf("expected cache expiration timestamp not to be modified"))

	// Forced cache-miss after expiry.
	// NOTE(achilleasa): this will trigger a "advancing a clock with nothing waiting"
	// warning but that's a false positive; we just want to advance the clock
	// to test the cache expiry logic.
	clk.Advance(11 * time.Minute)
	_, err = s.Env.getAllInstanceTypes(s.CallCtx, clk)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.Env.instCacheExpireAt.After(cacheExpAt), jc.IsTrue, gc.Commentf("expected cache expiration to be updated"))
	c.Assert(s.Env.instCacheExpireAt.After(clk.Now()), jc.IsTrue, gc.Commentf("expected cache expiration to be in the future"))
}
