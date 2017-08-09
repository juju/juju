// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodelrelations_test

import (
	"time"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/macaroon.v1"

	"github.com/juju/juju/api/crossmodelrelations"
	coretesting "github.com/juju/juju/testing"
	"gopkg.in/macaroon-bakery.v1/bakery/checkers"
)

const longerThanExpiryTime = 11 * time.Minute

var _ = gc.Suite(&MacaroonCacheSuite{})

type MacaroonCacheSuite struct {
	coretesting.BaseSuite
}

func (s *MacaroonCacheSuite) TestGetMacaroonMissing(c *gc.C) {
	cache := crossmodelrelations.NewMacaroonCache(testing.NewClock(time.Now()))
	_, ok := cache.Get("missing")
	c.Assert(ok, jc.IsFalse)
}

func (s *MacaroonCacheSuite) TestGetMacaroon(c *gc.C) {
	cache := crossmodelrelations.NewMacaroonCache(testing.NewClock(time.Now()))
	mac, err := macaroon.New(nil, "", "")
	c.Assert(err, jc.ErrorIsNil)
	cache.Upsert("token", macaroon.Slice{mac})
	ms, ok := cache.Get("token")
	c.Assert(ok, jc.IsTrue)
	c.Assert(ms, jc.DeepEquals, macaroon.Slice{mac})
}

func (s *MacaroonCacheSuite) TestGetMacaroonNotExpired(c *gc.C) {
	clock := testing.NewClock(time.Now())
	cache := crossmodelrelations.NewMacaroonCache(clock)

	mac, err := macaroon.New(nil, "", "")
	cav := checkers.TimeBeforeCaveat(clock.Now().Add(10 * time.Second))
	mac.AddFirstPartyCaveat(cav.Condition)
	c.Assert(err, jc.ErrorIsNil)

	cache.Upsert("token", macaroon.Slice{mac})
	clock.WaitAdvance(9*time.Second, coretesting.ShortWait, 1)

	ms, ok := cache.Get("token")
	c.Assert(ok, jc.IsTrue)
	c.Assert(ms, jc.DeepEquals, macaroon.Slice{mac})
}

func (s *MacaroonCacheSuite) TestGetMacaroonExpiredBeforeCleanup(c *gc.C) {
	clock := testing.NewClock(time.Now())
	cache := crossmodelrelations.NewMacaroonCache(clock)

	mac, err := macaroon.New(nil, "", "")
	cav := checkers.TimeBeforeCaveat(clock.Now().Add(10 * time.Second))
	mac.AddFirstPartyCaveat(cav.Condition)
	c.Assert(err, jc.ErrorIsNil)

	cache.Upsert("token", macaroon.Slice{mac})
	clock.WaitAdvance(20*time.Second, coretesting.ShortWait, 1)

	_, ok := cache.Get("token")
	c.Assert(ok, jc.IsFalse)
}

func (s *MacaroonCacheSuite) TestGetMacaroonAfterCleanup(c *gc.C) {
	clock := testing.NewClock(time.Now())
	cache := crossmodelrelations.NewMacaroonCache(clock)

	mac, err := macaroon.New(nil, "", "")
	cav := checkers.TimeBeforeCaveat(clock.Now().Add(60 * time.Minute))
	mac.AddFirstPartyCaveat(cav.Condition)
	c.Assert(err, jc.ErrorIsNil)

	cache.Upsert("token", macaroon.Slice{mac})
	clock.WaitAdvance(longerThanExpiryTime, coretesting.ShortWait, 1)

	ms, ok := cache.Get("token")
	c.Assert(ok, jc.IsTrue)
	c.Assert(ms, jc.DeepEquals, macaroon.Slice{mac})
}

func (s *MacaroonCacheSuite) TestMacaroonRemovedByCleanup(c *gc.C) {
	clock := testing.NewClock(time.Now())
	cache := crossmodelrelations.NewMacaroonCache(clock)

	mac, err := macaroon.New(nil, "", "")
	cav := checkers.TimeBeforeCaveat(clock.Now().Add(2 * time.Minute))
	mac.AddFirstPartyCaveat(cav.Condition)
	c.Assert(err, jc.ErrorIsNil)

	cache.Upsert("token", macaroon.Slice{mac})
	clock.WaitAdvance(longerThanExpiryTime, coretesting.ShortWait, 1)

	_, ok := cache.Get("token")
	c.Assert(ok, jc.IsFalse)
}

func (s *MacaroonCacheSuite) TestCleanupIgnoresMacaroonsWithoutTimeBefore(c *gc.C) {
	clock := testing.NewClock(time.Now())
	cache := crossmodelrelations.NewMacaroonCache(clock)

	mac, err := macaroon.New(nil, "", "")
	c.Assert(err, jc.ErrorIsNil)

	cache.Upsert("token", macaroon.Slice{mac})
	clock.WaitAdvance(longerThanExpiryTime, coretesting.ShortWait, 1)

	ms, ok := cache.Get("token")
	c.Assert(ok, jc.IsTrue)
	c.Assert(ms, jc.DeepEquals, macaroon.Slice{mac})
}
