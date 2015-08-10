// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package schedule_test

import (
	"time"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/storageprovisioner/internal/schedule"
)

type scheduleSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&scheduleSuite{})

func (*scheduleSuite) TestNextNoEvents(c *gc.C) {
	s := schedule.NewSchedule(coretesting.NewClock(time.Time{}))
	next := s.Next()
	c.Assert(next, gc.IsNil)
}

func (*scheduleSuite) TestNext(c *gc.C) {
	clock := coretesting.NewClock(time.Time{})
	now := clock.Now()
	s := schedule.NewSchedule(clock)

	s.Add("k0", "v0", now.Add(3*time.Second))
	s.Add("k1", "v1", now.Add(1500*time.Millisecond))
	s.Add("k2", "v2", now.Add(2*time.Second))
	s.Add("k3", "v3", now.Add(2500*time.Millisecond))

	assertNextOp(c, s, clock, 1500*time.Millisecond)
	clock.Advance(1500 * time.Millisecond)
	assertReady(c, s, clock, "v1")

	clock.Advance(500 * time.Millisecond)
	assertNextOp(c, s, clock, 0)
	assertReady(c, s, clock, "v2")

	s.Remove("k3")

	clock.Advance(2 * time.Second) // T+4
	assertNextOp(c, s, clock, 0)
	assertReady(c, s, clock, "v0")
}

func (*scheduleSuite) TestReadyNoEvents(c *gc.C) {
	s := schedule.NewSchedule(coretesting.NewClock(time.Time{}))
	ready := s.Ready(time.Now())
	c.Assert(ready, gc.HasLen, 0)
}

func (*scheduleSuite) TestAdd(c *gc.C) {
	clock := coretesting.NewClock(time.Time{})
	now := clock.Now()
	s := schedule.NewSchedule(clock)

	s.Add("k0", "v0", now.Add(3*time.Second))
	s.Add("k1", "v1", now.Add(1500*time.Millisecond))
	s.Add("k2", "v2", now.Add(2*time.Second))

	clock.Advance(time.Second) // T+1
	assertReady(c, s, clock /* nothing */)

	clock.Advance(time.Second) // T+2
	assertReady(c, s, clock, "v1", "v2")
	assertReady(c, s, clock /* nothing */)

	clock.Advance(500 * time.Millisecond) // T+2.5
	assertReady(c, s, clock /* nothing */)

	clock.Advance(time.Second) // T+3.5
	assertReady(c, s, clock, "v0")
}

func (*scheduleSuite) TestRemove(c *gc.C) {
	clock := coretesting.NewClock(time.Time{})
	now := clock.Now()
	s := schedule.NewSchedule(clock)

	s.Add("k0", "v0", now.Add(3*time.Second))
	s.Add("k1", "v1", now.Add(2*time.Second))
	s.Remove("k0")
	assertReady(c, s, clock /* nothing */)

	clock.Advance(3 * time.Second)
	assertReady(c, s, clock, "v1")
}

func (*scheduleSuite) TestRemoveKeyNotFound(c *gc.C) {
	s := schedule.NewSchedule(coretesting.NewClock(time.Time{}))
	s.Remove("0") // does not explode
}

func assertNextOp(c *gc.C, s *schedule.Schedule, clock *coretesting.Clock, d time.Duration) {
	next := s.Next()
	c.Assert(next, gc.NotNil)
	if d > 0 {
		select {
		case <-next:
			c.Fatal("Next channel signalled too soon")
		default:
		}
	}

	// temporarily move time forward
	clock.Advance(d)
	defer clock.Advance(-d)

	select {
	case _, ok := <-next:
		c.Assert(ok, jc.IsTrue)
		// the time value is unimportant to us
	default:
		c.Fatal("Next channel not signalled")
	}
}

func assertReady(c *gc.C, s *schedule.Schedule, clock *coretesting.Clock, expect ...interface{}) {
	ready := s.Ready(clock.Now())
	c.Assert(ready, jc.DeepEquals, expect)
}
