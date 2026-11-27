// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secrettriggerschedule_test

import (
	"fmt"
	"time"

	"github.com/juju/clock/testclock"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/secrets"
	"github.com/juju/juju/internal/worker/secrettriggerschedule"
	"github.com/juju/juju/internal/worker/secrettriggerschedule/scheduletest"
)

type ScheduleSuite struct{}

var _ = gc.Suite(&ScheduleSuite{})

func (s *ScheduleSuite) TestTriggerScheduleAfterFire(c *gc.C) {
	now := time.Now().UTC()
	sched := secrettriggerschedule.NewTriggerSchedule(now)
	c.Check(sched.TriggerTime, gc.Equals, now)
	c.Check(sched.NextTime, gc.Equals, now)
	c.Check(sched.Fired, jc.IsFalse)
	c.Check(sched.RetryCount, gc.Equals, 0)
	c.Check(sched.RetriesExhausted(), jc.IsFalse)

	logger := &fakeLogger{}
	bumped := sched.AfterFire(logger, "secret rotation", "secret:abc", secrets.RotateRetryDelay, now)
	c.Check(sched.Fired, jc.IsFalse)
	c.Check(sched.RetryCount, gc.Equals, 0)
	c.Check(sched.NextTime, gc.Equals, now)
	sched = bumped
	c.Check(sched.Fired, jc.IsTrue)
	c.Check(sched.RetryCount, gc.Equals, 0)
	c.Check(sched.NextTime, gc.Equals, now.Add(secrets.RotateRetryDelay))
	c.Check(sched.RetriesExhausted(), jc.IsFalse)

	// Retries run at fixed intervals of the base delay.
	for attempt := 1; attempt <= secrettriggerschedule.MaxTriggerRetries; attempt++ {
		fireAt := sched.NextTime
		sched = sched.AfterFire(logger, "secret rotation", "secret:abc", secrets.RotateRetryDelay, fireAt)
		c.Check(sched.RetryCount, gc.Equals, attempt)
		c.Check(sched.NextTime, gc.Equals, fireAt.Add(secrets.RotateRetryDelay))
		if attempt < secrettriggerschedule.MaxTriggerRetries {
			c.Check(sched.RetriesExhausted(), jc.IsFalse)
		} else {
			c.Check(sched.RetriesExhausted(), jc.IsTrue)
		}
	}

	// Once MaxTriggerRetries is reached, RetryCount stays capped at
	// MaxTriggerRetries and RetriesExhausted remains true.
	fireAt := sched.NextTime
	sched = sched.AfterFire(logger, "secret rotation", "secret:abc", secrets.RotateRetryDelay, fireAt)
	c.Check(sched.RetryCount, gc.Equals, secrettriggerschedule.MaxTriggerRetries)
	c.Check(sched.RetriesExhausted(), jc.IsTrue)
	c.Check(sched.NextTime, gc.Equals, fireAt.Add(secrets.RotateRetryDelay))
}

func (s *ScheduleSuite) TestTriggerScheduleShouldIgnoreTime(c *gc.C) {
	now := time.Now().UTC()
	sched := secrettriggerschedule.NewTriggerSchedule(now.Add(2 * time.Hour))

	// Before first fire (!Fired), earlier and later times are both accepted.
	c.Check(sched.ShouldIgnoreTime(now.Add(time.Hour)), jc.IsFalse)
	c.Check(sched.ShouldIgnoreTime(now.Add(2*time.Hour)), jc.IsFalse)

	// After firing (Fired == true), the identical trigger time is ignored,
	// while both earlier (shortened schedule) and later trigger times are
	// accepted.
	sched = sched.AfterFire(&fakeLogger{}, "secret rotation", "secret:abc", secrets.RotateRetryDelay, now.Add(2*time.Hour))
	c.Check(sched.ShouldIgnoreTime(now.Add(2*time.Hour)), jc.IsTrue)
	c.Check(sched.ShouldIgnoreTime(now.Add(time.Hour)), jc.IsFalse)
	c.Check(sched.ShouldIgnoreTime(now.Add(3*time.Hour)), jc.IsFalse)
}

func (s *ScheduleSuite) TestAfterFireLogsFirstFireAndRetries(c *gc.C) {
	now := time.Now().UTC()
	sched := secrettriggerschedule.NewTriggerSchedule(now)
	logger := &fakeLogger{}

	// The first fire logs at Info level.
	sched = sched.AfterFire(logger, "secret rotation", "secret:abc", secrets.RotateRetryDelay, now)
	c.Check(logger.infos, gc.HasLen, 1)
	c.Check(logger.debugs, gc.HasLen, 0)
	c.Check(sched.Fired, jc.IsTrue)

	// Subsequent fires log at Debug level.
	for range secrettriggerschedule.MaxTriggerRetries {
		sched = sched.AfterFire(logger, "secret rotation", "secret:abc", secrets.RotateRetryDelay, sched.NextTime)
	}
	c.Check(logger.debugs, gc.HasLen, secrettriggerschedule.MaxTriggerRetries)

	// Fires past the retry cap keep logging at Debug level.
	sched = sched.AfterFire(logger, "secret rotation", "secret:abc", secrets.RotateRetryDelay, sched.NextTime)
	c.Check(logger.debugs, gc.HasLen, secrettriggerschedule.MaxTriggerRetries+1)
}

func (s *ScheduleSuite) TestSoonestTimeSkipsZero(c *gc.C) {
	getNext := func(s secrettriggerschedule.TriggerSchedule) time.Time {
		return s.NextTime
	}
	c.Check(secrettriggerschedule.SoonestTime[secrettriggerschedule.TriggerSchedule](nil, getNext).IsZero(), jc.IsTrue)
	c.Check(secrettriggerschedule.SoonestTime(map[string]secrettriggerschedule.TriggerSchedule{}, getNext).IsZero(), jc.IsTrue)

	now := time.Now().UTC()
	entries := map[string]secrettriggerschedule.TriggerSchedule{
		"zero":    {},
		"pending": secrettriggerschedule.NewTriggerSchedule(now.Add(2 * time.Hour)),
	}
	soonest := secrettriggerschedule.SoonestTime(entries, getNext)
	c.Check(soonest, gc.Equals, now.Add(2*time.Hour))

	delete(entries, "pending")
	soonest = secrettriggerschedule.SoonestTime(entries, getNext)
	c.Check(soonest.IsZero(), jc.IsTrue)
}

func (s *ScheduleSuite) TestScheduleTimer(c *gc.C) {
	clk := testclock.NewClock(time.Now())
	now := clk.Now()

	// Unarmed timer with zero trigger time is a no-op.
	timer, scheduled, changed := secrettriggerschedule.ScheduleTimer(clk, nil, time.Time{}, time.Time{})
	c.Check(timer, gc.IsNil)
	c.Check(scheduled.IsZero(), jc.IsTrue)
	c.Check(changed, jc.IsFalse)
	scheduletest.AssertNoTimerArmed(c, clk)

	entries := map[string]time.Time{
		"a": now.Add(2 * time.Hour),
		"b": now.Add(time.Hour),
	}
	soonest := secrettriggerschedule.SoonestTime(entries, func(t time.Time) time.Time { return t })
	c.Check(soonest, gc.Equals, now.Add(time.Hour))

	timer, scheduled, changed = secrettriggerschedule.ScheduleTimer(clk, nil, scheduled, soonest)
	c.Assert(timer, gc.NotNil)
	c.Check(scheduled, gc.Equals, now.Add(time.Hour))
	c.Check(changed, jc.IsTrue)
	scheduletest.WaitTimerArmed(c, clk)

	// Equal instant (even in a different location) leaves the timer unchanged.
	otherLoc := scheduled.In(time.FixedZone("UTC+2", int((2 * time.Hour).Seconds())))
	sameTimer, sameScheduled, changed := secrettriggerschedule.ScheduleTimer(clk, timer, scheduled, otherLoc)
	c.Check(sameTimer, gc.Equals, timer)
	c.Check(sameScheduled, gc.Equals, scheduled)
	c.Check(changed, jc.IsFalse)
	scheduletest.AssertNoTimerArmed(c, clk)

	// Resetting a timer that has already expired with an unread tick drains the
	// buffered tick before re-arming.
	clk.Advance(time.Hour)
	now = clk.Now()
	soonest = now.Add(time.Hour)
	timer, scheduled, changed = secrettriggerschedule.ScheduleTimer(clk, timer, scheduled, soonest)
	c.Assert(timer, gc.NotNil)
	c.Check(scheduled, gc.Equals, soonest)
	c.Check(changed, jc.IsTrue)
	scheduletest.WaitTimerArmed(c, clk)
	select {
	case <-timer.Chan():
		c.Fatal("expected buffered timer tick to be drained on reset")
	default:
	}

	// Zero current with non-nil timer (post-fire state) re-arms for the same
	// deadline.
	timer, scheduled, changed = secrettriggerschedule.ScheduleTimer(clk, timer, time.Time{}, soonest)
	c.Assert(timer, gc.NotNil)
	c.Check(scheduled, gc.Equals, soonest)
	c.Check(changed, jc.IsTrue)
	scheduletest.WaitTimerArmed(c, clk)

	// Past trigger time is set to now (zero delay).
	timer, scheduled, changed = secrettriggerschedule.ScheduleTimer(clk, timer, scheduled, now.Add(-time.Minute))
	c.Assert(timer, gc.NotNil)
	c.Check(scheduled, gc.Equals, now)
	c.Check(changed, jc.IsTrue)
	scheduletest.WaitTimerArmed(c, clk)

	// Zero trigger time stops the timer and returns nil.
	timer, scheduled, changed = secrettriggerschedule.ScheduleTimer(clk, timer, scheduled, time.Time{})
	c.Check(timer, gc.IsNil)
	c.Check(scheduled.IsZero(), jc.IsTrue)
	c.Check(changed, jc.IsTrue)
	scheduletest.AssertNoTimerArmed(c, clk)
}

func (s *ScheduleSuite) TestScheduleAlarm(c *gc.C) {
	clk := testclock.NewClock(time.Now())
	now := clk.Now()

	// Unarmed alarm with zero trigger time is a no-op.
	alarm, scheduled, changed := secrettriggerschedule.ScheduleAlarm(clk, nil, time.Time{}, time.Time{})
	c.Check(alarm, gc.IsNil)
	c.Check(scheduled.IsZero(), jc.IsTrue)
	c.Check(changed, jc.IsFalse)
	scheduletest.AssertNoTimerArmed(c, clk)

	deadline := now.Add(time.Hour)
	alarm, scheduled, changed = secrettriggerschedule.ScheduleAlarm(clk, nil, scheduled, deadline)
	c.Assert(alarm, gc.NotNil)
	c.Check(scheduled, gc.Equals, deadline)
	c.Check(changed, jc.IsTrue)
	scheduletest.WaitTimerArmed(c, clk)

	// Equal instant (even in a different location) leaves the alarm unchanged.
	otherLoc := scheduled.In(time.FixedZone("UTC+2", int((2 * time.Hour).Seconds())))
	sameAlarm, sameScheduled, changed := secrettriggerschedule.ScheduleAlarm(clk, alarm, scheduled, otherLoc)
	c.Check(sameAlarm, gc.Equals, alarm)
	c.Check(sameScheduled, gc.Equals, scheduled)
	c.Check(changed, jc.IsFalse)
	scheduletest.AssertNoTimerArmed(c, clk)

	// Resetting an alarm that has already expired with an unread tick drains
	// the buffered tick before re-arming.
	clk.Advance(time.Hour)
	now = clk.Now()
	deadline = now.Add(time.Hour)
	alarm, scheduled, changed = secrettriggerschedule.ScheduleAlarm(clk, alarm, scheduled, deadline)
	c.Assert(alarm, gc.NotNil)
	c.Check(scheduled, gc.Equals, deadline)
	c.Check(changed, jc.IsTrue)
	scheduletest.WaitTimerArmed(c, clk)
	select {
	case <-alarm.Chan():
		c.Fatal("expected buffered alarm tick to be drained on reset")
	default:
	}

	// Zero current with non-nil alarm (post-fire state) re-arms for the same
	// deadline.
	alarm, scheduled, changed = secrettriggerschedule.ScheduleAlarm(clk, alarm, time.Time{}, deadline)
	c.Assert(alarm, gc.NotNil)
	c.Check(scheduled, gc.Equals, deadline)
	c.Check(changed, jc.IsTrue)
	scheduletest.WaitTimerArmed(c, clk)

	// Past trigger time is set to now.
	alarm, scheduled, changed = secrettriggerschedule.ScheduleAlarm(clk, alarm, scheduled, now.Add(-time.Minute))
	c.Assert(alarm, gc.NotNil)
	c.Check(scheduled, gc.Equals, now)
	c.Check(changed, jc.IsTrue)
	scheduletest.WaitTimerArmed(c, clk)

	// Zero trigger time stops the alarm and returns nil.
	alarm, scheduled, changed = secrettriggerschedule.ScheduleAlarm(clk, alarm, scheduled, time.Time{})
	c.Check(alarm, gc.IsNil)
	c.Check(scheduled.IsZero(), jc.IsTrue)
	c.Check(changed, jc.IsTrue)
	scheduletest.AssertNoTimerArmed(c, clk)
}

type fakeLogger struct {
	debugs []string
	infos  []string
}

func (l *fakeLogger) Debugf(format string, args ...any) {
	l.debugs = append(l.debugs, fmt.Sprintf(format, args...))
}

func (l *fakeLogger) Infof(format string, args ...any) {
	l.infos = append(l.infos, fmt.Sprintf(format, args...))
}
