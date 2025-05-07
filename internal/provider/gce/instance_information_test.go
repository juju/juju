// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce

import (
	"context"
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"

	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/internal/provider/gce/google"
)

type instanceInformationSuite struct {
	BaseSuite
}

var _ = tc.Suite(&instanceInformationSuite{})

func (s *instanceInformationSuite) TestInstanceTypesCacheExpiration(c *tc.C) {
	zone := google.NewZone("a-zone", google.StatusUp, "", "")
	s.FakeConn.Zones = []google.AvailabilityZone{zone}

	now := time.Now()
	clk := testclock.NewClock(now)
	ctx := context.Background()
	allInstTypes, err := s.Env.getAllInstanceTypes(ctx, clk)
	c.Assert(err, jc.ErrorIsNil)

	// Cache miss
	cacheExpAt := s.Env.instCacheExpireAt
	c.Assert(cacheExpAt.After(now), jc.IsTrue, tc.Commentf("expected a cache expiration time to be set"))

	// Cache hit
	cachedInstTypes, err := s.Env.getAllInstanceTypes(ctx, clk)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(allInstTypes, tc.DeepEquals, cachedInstTypes, tc.Commentf("expected to get cached instance list"))
	c.Assert(s.Env.instCacheExpireAt, tc.Equals, cacheExpAt, tc.Commentf("expected cache expiration timestamp not to be modified"))

	// Forced cache-miss after expiry.
	// NOTE(achilleasa): this will trigger a "advancing a clock with nothing waiting"
	// warning but that's a false positive; we just want to advance the clock
	// to test the cache expiry logic.
	clk.Advance(11 * time.Minute)
	_, err = s.Env.getAllInstanceTypes(ctx, clk)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.Env.instCacheExpireAt.After(cacheExpAt), jc.IsTrue, tc.Commentf("expected cache expiration to be updated"))
	c.Assert(s.Env.instCacheExpireAt.After(clk.Now()), jc.IsTrue, tc.Commentf("expected cache expiration to be in the future"))
}

func (s *instanceInformationSuite) TestEnsureDefaultConstraints(c *tc.C) {
	// Fill default cores and mem.
	cons := constraints.Value{}
	c.Assert(cons.String(), tc.Equals, ``)
	cons = ensureDefaultConstraints(cons)
	c.Assert(cons.String(), tc.Equals, `cores=2`)

	var err error
	// Do not fill default cores and mem if instance type is provided.
	cons, err = constraints.Parse(`instance-type=e2-medium`)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cons.String(), tc.Equals, `instance-type=e2-medium`)
	cons = ensureDefaultConstraints(cons)
	c.Assert(cons.String(), tc.Equals, `instance-type=e2-medium`)

	// Do not fill default cores and mem if cores or/and mem are provided.
	cons, err = constraints.Parse(`cores=1 mem=1024M`) // smaller than defaults
	c.Assert(err, jc.ErrorIsNil)
	cons = ensureDefaultConstraints(cons)
	c.Assert(cons.String(), tc.Equals, `cores=1 mem=1024M`)

	cons, err = constraints.Parse(`cores=4 mem=4096M`)
	c.Assert(err, jc.ErrorIsNil)
	cons = ensureDefaultConstraints(cons)
	c.Assert(cons.String(), tc.Equals, `cores=4 mem=4096M`)

	cons, err = constraints.Parse(`cores=4`)
	c.Assert(err, jc.ErrorIsNil)
	cons = ensureDefaultConstraints(cons)
	c.Assert(cons.String(), tc.Equals, `cores=4`)

	cons, err = constraints.Parse(`mem=4096M`)
	c.Assert(err, jc.ErrorIsNil)
	cons = ensureDefaultConstraints(cons)
	c.Assert(cons.String(), tc.Equals, `cores=2 mem=4096M`)
}
