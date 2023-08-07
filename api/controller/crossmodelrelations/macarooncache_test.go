// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodelrelations_test

import (
	"time"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery/checkers"
	"github.com/juju/clock/testclock"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/api/controller/crossmodelrelations"
	jujutesting "github.com/juju/juju/juju/testing"
	coretesting "github.com/juju/juju/testing"
)

const longerThanExpiryTime = 11 * time.Minute

var _ = gc.Suite(&MacaroonCacheSuite{})

type MacaroonCacheSuite struct {
	coretesting.BaseSuite
}

func (s *MacaroonCacheSuite) TestGetMacaroonMissing(c *gc.C) {
	cache := crossmodelrelations.NewMacaroonCache(testclock.NewClock(time.Now()))
	_, ok := cache.Get("missing")
	c.Assert(ok, jc.IsFalse)
}

func (s *MacaroonCacheSuite) TestGetMacaroon(c *gc.C) {
	cache := crossmodelrelations.NewMacaroonCache(testclock.NewClock(time.Now()))
	mac, err := jujutesting.NewMacaroon("id")
	c.Assert(err, jc.ErrorIsNil)
	cache.Upsert("token", macaroon.Slice{mac})
	ms, ok := cache.Get("token")
	c.Assert(ok, jc.IsTrue)
	c.Assert(ms, jc.DeepEquals, macaroon.Slice{mac})
}

func (s *MacaroonCacheSuite) TestGetMacaroonNotExpired(c *gc.C) {
	clock := testclock.NewClock(time.Now())
	cache := crossmodelrelations.NewMacaroonCache(clock)

	mac, err := jujutesting.NewMacaroon("id")
	cav := checkers.TimeBeforeCaveat(clock.Now().Add(10 * time.Second))
	mac.AddFirstPartyCaveat([]byte(cav.Condition))
	c.Assert(err, jc.ErrorIsNil)

	cache.Upsert("token", macaroon.Slice{mac})
	clock.WaitAdvance(9*time.Second, coretesting.ShortWait, 1)

	ms, ok := cache.Get("token")
	c.Assert(ok, jc.IsTrue)
	c.Assert(ms, jc.DeepEquals, macaroon.Slice{mac})
}

func (s *MacaroonCacheSuite) TestGetMacaroonExpiredBeforeCleanup(c *gc.C) {
	clock := testclock.NewClock(time.Now())
	cache := crossmodelrelations.NewMacaroonCache(clock)

	mac, err := jujutesting.NewMacaroon("id")
	cav := checkers.TimeBeforeCaveat(clock.Now().Add(10 * time.Second))
	mac.AddFirstPartyCaveat([]byte(cav.Condition))
	c.Assert(err, jc.ErrorIsNil)

	cache.Upsert("token", macaroon.Slice{mac})
	clock.WaitAdvance(20*time.Second, coretesting.ShortWait, 1)

	_, ok := cache.Get("token")
	c.Assert(ok, jc.IsFalse)
}

func (s *MacaroonCacheSuite) TestGetMacaroonAfterCleanup(c *gc.C) {
	clock := testclock.NewClock(time.Now())
	cache := crossmodelrelations.NewMacaroonCache(clock)

	mac, err := jujutesting.NewMacaroon("id")
	cav := checkers.TimeBeforeCaveat(clock.Now().Add(60 * time.Minute))
	mac.AddFirstPartyCaveat([]byte(cav.Condition))
	c.Assert(err, jc.ErrorIsNil)

	cache.Upsert("token", macaroon.Slice{mac})
	clock.WaitAdvance(longerThanExpiryTime, coretesting.ShortWait, 1)

	ms, ok := cache.Get("token")
	c.Assert(ok, jc.IsTrue)
	c.Assert(ms, jc.DeepEquals, macaroon.Slice{mac})
}

func (s *MacaroonCacheSuite) TestMacaroonRemovedByCleanup(c *gc.C) {
	clock := testclock.NewClock(time.Now())
	cache := crossmodelrelations.NewMacaroonCache(clock)

	mac, err := jujutesting.NewMacaroon("id")
	cav := checkers.TimeBeforeCaveat(clock.Now().Add(2 * time.Minute))
	mac.AddFirstPartyCaveat([]byte(cav.Condition))
	c.Assert(err, jc.ErrorIsNil)

	cache.Upsert("token", macaroon.Slice{mac})
	clock.WaitAdvance(longerThanExpiryTime, coretesting.ShortWait, 1)

	_, ok := cache.Get("token")
	c.Assert(ok, jc.IsFalse)
}

func (s *MacaroonCacheSuite) TestCleanupIgnoresMacaroonsWithoutTimeBefore(c *gc.C) {
	clock := testclock.NewClock(time.Now())
	cache := crossmodelrelations.NewMacaroonCache(clock)

	mac, err := jujutesting.NewMacaroon("id")
	c.Assert(err, jc.ErrorIsNil)

	cache.Upsert("token", macaroon.Slice{mac})
	clock.WaitAdvance(longerThanExpiryTime, coretesting.ShortWait, 1)

	ms, ok := cache.Get("token")
	c.Assert(ok, jc.IsTrue)
	c.Assert(ms, jc.DeepEquals, macaroon.Slice{mac})
}
