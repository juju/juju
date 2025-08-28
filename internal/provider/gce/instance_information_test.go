// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce

import (
	"testing"
	"time"

	"cloud.google.com/go/compute/apiv1/computepb"
	"github.com/juju/clock/testclock"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/core/constraints"
)

type instanceInformationSuite struct {
	BaseSuite
}

func TestInstanceInformationSuite(t *testing.T) {
	tc.Run(t, &instanceInformationSuite{})
}
func (s *instanceInformationSuite) TestInstanceTypesCacheExpiration(c *tc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	env := s.SetupEnv(c, s.MockService)

	s.MockService.EXPECT().AvailabilityZones(gomock.Any(), "us-east1").
		Return([]*computepb.Zone{{Name: ptr("us-east1")}}, nil).Times(3)

	now := time.Now()
	clk := testclock.NewClock(now)
	allInstTypes, err := env.getAllInstanceTypes(c.Context(), clk)
	c.Assert(err, tc.ErrorIsNil)

	// Cache miss
	cacheExpAt := env.instCacheExpireAt
	c.Assert(cacheExpAt.After(now), tc.IsTrue, tc.Commentf("expected a cache expiration time to be set"))

	// Cache hit
	cachedInstTypes, err := env.getAllInstanceTypes(c.Context(), clk)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(allInstTypes, tc.DeepEquals, cachedInstTypes, tc.Commentf("expected to get cached instance list"))
	c.Assert(env.instCacheExpireAt, tc.Equals, cacheExpAt, tc.Commentf("expected cache expiration timestamp not to be modified"))

	// Forced cache-miss after expiry.
	// NOTE(achilleasa): this will trigger a "advancing a clock with nothing waiting"
	// warning but that's a false positive; we just want to advance the clock
	// to test the cache expiry logic.
	clk.Advance(11 * time.Minute)
	_, err = env.getAllInstanceTypes(c.Context(), clk)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(env.instCacheExpireAt.After(cacheExpAt), tc.IsTrue, tc.Commentf("expected cache expiration to be updated"))
	c.Assert(env.instCacheExpireAt.After(clk.Now()), tc.IsTrue, tc.Commentf("expected cache expiration to be in the future"))
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
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cons.String(), tc.Equals, `instance-type=e2-medium`)
	cons = ensureDefaultConstraints(cons)
	c.Assert(cons.String(), tc.Equals, `instance-type=e2-medium`)

	// Do not fill default cores and mem if cores or/and mem are provided.
	cons, err = constraints.Parse(`cores=1 mem=1024M`) // smaller than defaults
	c.Assert(err, tc.ErrorIsNil)
	cons = ensureDefaultConstraints(cons)
	c.Assert(cons.String(), tc.Equals, `cores=1 mem=1024M`)

	cons, err = constraints.Parse(`cores=4 mem=4096M`)
	c.Assert(err, tc.ErrorIsNil)
	cons = ensureDefaultConstraints(cons)
	c.Assert(cons.String(), tc.Equals, `cores=4 mem=4096M`)

	cons, err = constraints.Parse(`cores=4`)
	c.Assert(err, tc.ErrorIsNil)
	cons = ensureDefaultConstraints(cons)
	c.Assert(cons.String(), tc.Equals, `cores=4`)

	cons, err = constraints.Parse(`mem=4096M`)
	c.Assert(err, tc.ErrorIsNil)
	cons = ensureDefaultConstraints(cons)
	c.Assert(cons.String(), tc.Equals, `cores=2 mem=4096M`)
}
