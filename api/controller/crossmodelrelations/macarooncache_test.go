// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodelrelations_test

import (
	"testing"
	"time"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery/checkers"
	"github.com/juju/clock/testclock"
	"github.com/juju/tc"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/api/controller/crossmodelrelations"
	coretesting "github.com/juju/juju/internal/testing"
	jujutesting "github.com/juju/juju/juju/testing"
)

const longerThanExpiryTime = 11 * time.Minute

func TestMacaroonCacheSuite(t *testing.T) {
	tc.Run(t, &MacaroonCacheSuite{})
}

type MacaroonCacheSuite struct {
	coretesting.BaseSuite
}

func (s *MacaroonCacheSuite) TestGetMacaroonMissing(c *tc.C) {
	cache := crossmodelrelations.NewMacaroonCache(testclock.NewClock(time.Now()))
	_, ok := cache.Get("missing")
	c.Assert(ok, tc.IsFalse)
}

func (s *MacaroonCacheSuite) TestGetMacaroon(c *tc.C) {
	cache := crossmodelrelations.NewMacaroonCache(testclock.NewClock(time.Now()))
	mac, err := jujutesting.NewMacaroon("id")
	c.Assert(err, tc.ErrorIsNil)
	cache.Upsert("token", macaroon.Slice{mac})
	ms, ok := cache.Get("token")
	c.Assert(ok, tc.IsTrue)
	c.Assert(ms, tc.DeepEquals, macaroon.Slice{mac})
}

func (s *MacaroonCacheSuite) TestGetMacaroonNotExpired(c *tc.C) {
	clock := testclock.NewClock(time.Now())
	cache := crossmodelrelations.NewMacaroonCache(clock)

	mac, err := jujutesting.NewMacaroon("id")
	cav := checkers.TimeBeforeCaveat(clock.Now().Add(10 * time.Second))
	mac.AddFirstPartyCaveat([]byte(cav.Condition))
	c.Assert(err, tc.ErrorIsNil)

	cache.Upsert("token", macaroon.Slice{mac})
	clock.WaitAdvance(9*time.Second, coretesting.ShortWait, 1)

	ms, ok := cache.Get("token")
	c.Assert(ok, tc.IsTrue)
	c.Assert(ms, tc.DeepEquals, macaroon.Slice{mac})
}

func (s *MacaroonCacheSuite) TestGetMacaroonExpiredBeforeCleanup(c *tc.C) {
	clock := testclock.NewClock(time.Now())
	cache := crossmodelrelations.NewMacaroonCache(clock)

	mac, err := jujutesting.NewMacaroon("id")
	cav := checkers.TimeBeforeCaveat(clock.Now().Add(10 * time.Second))
	mac.AddFirstPartyCaveat([]byte(cav.Condition))
	c.Assert(err, tc.ErrorIsNil)

	cache.Upsert("token", macaroon.Slice{mac})
	clock.WaitAdvance(20*time.Second, coretesting.ShortWait, 1)

	_, ok := cache.Get("token")
	c.Assert(ok, tc.IsFalse)
}

func (s *MacaroonCacheSuite) TestGetMacaroonAfterCleanup(c *tc.C) {
	clock := testclock.NewClock(time.Now())
	cache := crossmodelrelations.NewMacaroonCache(clock)

	mac, err := jujutesting.NewMacaroon("id")
	cav := checkers.TimeBeforeCaveat(clock.Now().Add(60 * time.Minute))
	mac.AddFirstPartyCaveat([]byte(cav.Condition))
	c.Assert(err, tc.ErrorIsNil)

	cache.Upsert("token", macaroon.Slice{mac})
	clock.WaitAdvance(longerThanExpiryTime, coretesting.ShortWait, 1)

	ms, ok := cache.Get("token")
	c.Assert(ok, tc.IsTrue)
	c.Assert(ms, tc.DeepEquals, macaroon.Slice{mac})
}

func (s *MacaroonCacheSuite) TestMacaroonRemovedByCleanup(c *tc.C) {
	clock := testclock.NewClock(time.Now())
	cache := crossmodelrelations.NewMacaroonCache(clock)

	mac, err := jujutesting.NewMacaroon("id")
	cav := checkers.TimeBeforeCaveat(clock.Now().Add(2 * time.Minute))
	mac.AddFirstPartyCaveat([]byte(cav.Condition))
	c.Assert(err, tc.ErrorIsNil)

	cache.Upsert("token", macaroon.Slice{mac})
	clock.WaitAdvance(longerThanExpiryTime, coretesting.ShortWait, 1)

	_, ok := cache.Get("token")
	c.Assert(ok, tc.IsFalse)
}

func (s *MacaroonCacheSuite) TestCleanupIgnoresMacaroonsWithoutTimeBefore(c *tc.C) {
	clock := testclock.NewClock(time.Now())
	cache := crossmodelrelations.NewMacaroonCache(clock)

	mac, err := jujutesting.NewMacaroon("id")
	c.Assert(err, tc.ErrorIsNil)

	cache.Upsert("token", macaroon.Slice{mac})
	clock.WaitAdvance(longerThanExpiryTime, coretesting.ShortWait, 1)

	ms, ok := cache.Get("token")
	c.Assert(ok, tc.IsTrue)
	c.Assert(ms, tc.DeepEquals, macaroon.Slice{mac})
}
