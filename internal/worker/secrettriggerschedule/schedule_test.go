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
	"github.com/juju/juju/testing"
)

type ScheduleSuite struct{}

var _ = gc.Suite(&ScheduleSuite{})

func (s *ScheduleSuite) TestTriggerRetryDelay(c *gc.C) {
	c.Check(secrettriggerschedule.TriggerRetryDelay(secrets.RotateRetryDelay, 0), gc.Equals, 5*time.Minute)
	c.Check(secrettriggerschedule.TriggerRetryDelay(secrets.RotateRetryDelay, 1), gc.Equals, 10*time.Minute)
	c.Check(secrettriggerschedule.TriggerRetryDelay(secrets.RotateRetryDelay, 2), gc.Equals, 20*time.Minute)
	c.Check(secrettriggerschedule.TriggerRetryDelay(secrets.RotateRetryDelay, 3), gc.Equals, 40*time.Minute)
	c.Check(secrettriggerschedule.TriggerRetryDelay(secrets.RotateRetryDelay, 4), gc.Equals, time.Hour)
	c.Check(secrettriggerschedule.TriggerRetryDelay(secrets.RotateRetryDelay, 5), gc.Equals, time.Hour)
	c.Check(secrettriggerschedule.TriggerRetryDelay(secrettriggerschedule.MaxTriggerRetryDelay, 0), gc.Equals, secrettriggerschedule.MaxTriggerRetryDelay)
	c.Check(secrettriggerschedule.TriggerRetryDelay(secrettriggerschedule.MaxTriggerRetryDelay, 1), gc.Equals, secrettriggerschedule.MaxTriggerRetryDelay)
	c.Check(secrettriggerschedule.TriggerRetryDelay(2*time.Hour, 0), gc.Equals, secrettriggerschedule.MaxTriggerRetryDelay)
}

func (s *ScheduleSuite) TestTriggerScheduleBumped(c *gc.C) {
	now := time.Now().UTC()
	sched := secrettriggerschedule.NewTriggerSchedule(now)
	c.Check(sched.TriggerTime, gc.Equals, now)
	c.Check(sched.NextTime, gc.Equals, now)
	c.Check(sched.RetryCount, gc.Equals, 0)

	bumped := sched.Bumped(secrets.RotateRetryDelay, now)
	c.Check(sched.RetryCount, gc.Equals, 0)
	c.Check(sched.NextTime, gc.Equals, now)
	sched = bumped
	c.Check(sched.RetryCount, gc.Equals, 1)
	c.Check(sched.NextTime, gc.Equals, now.Add(5*time.Minute))

	for attempt := 1; attempt < secrettriggerschedule.MaxTriggerRetries; attempt++ {
		fireAt := sched.NextTime
		sched = sched.Bumped(secrets.RotateRetryDelay, fireAt)
		c.Check(sched.RetryCount, gc.Equals, attempt+1)
	}

	// Once MaxTriggerRetries is reached, RetryCount stays capped and retries
	// continue at MaxTriggerRetryDelay.
	fireAt := sched.NextTime
	sched = sched.Bumped(secrets.RotateRetryDelay, fireAt)
	c.Check(sched.RetryCount, gc.Equals, secrettriggerschedule.MaxTriggerRetries)
	c.Check(sched.NextTime, gc.Equals, fireAt.Add(secrettriggerschedule.MaxTriggerRetryDelay))
}

func (s *ScheduleSuite) TestTriggerScheduleShouldIgnoreTime(c *gc.C) {
	now := time.Now().UTC()
	sched := secrettriggerschedule.NewTriggerSchedule(now.Add(2 * time.Hour))

	// Before first fire (RetryCount == 0), earlier and later times are
	// both accepted.
	c.Check(sched.ShouldIgnoreTime(now.Add(time.Hour)), jc.IsFalse)
	c.Check(sched.ShouldIgnoreTime(now.Add(2*time.Hour)), jc.IsFalse)

	// After firing (RetryCount > 0), the identical trigger time is ignored,
	// while both earlier (shortened schedule) and later trigger times are
	// accepted.
	sched = sched.Bumped(secrets.RotateRetryDelay, now.Add(2*time.Hour))
	c.Check(sched.ShouldIgnoreTime(now.Add(2*time.Hour)), jc.IsTrue)
	c.Check(sched.ShouldIgnoreTime(now.Add(time.Hour)), jc.IsFalse)
	c.Check(sched.ShouldIgnoreTime(now.Add(3*time.Hour)), jc.IsFalse)
}

func (s *ScheduleSuite) TestFire(c *gc.C) {
	now := time.Now().UTC()
	sched := secrettriggerschedule.NewTriggerSchedule(now)
	logger := &fakeLogger{}

	sched = secrettriggerschedule.AfterFire(sched, logger, "secret rotation", "secret:abc", secrets.RotateRetryDelay, now)
	c.Check(logger.infos, gc.HasLen, 1)
	c.Check(logger.warnings, gc.HasLen, 0)
	c.Check(sched.RetryCount, gc.Equals, 1)
	c.Check(sched.NextTime, gc.Equals, now.Add(5*time.Minute))

	for attempt := 1; attempt < secrettriggerschedule.MaxTriggerRetries; attempt++ {
		fireAt := sched.NextTime
		sched = secrettriggerschedule.AfterFire(sched, logger, "secret rotation", "secret:abc", secrets.RotateRetryDelay, fireAt)
		c.Check(sched.RetryCount, gc.Equals, attempt+1)
	}
	c.Check(logger.warnings, gc.HasLen, secrettriggerschedule.MaxTriggerRetries-1)

	// Once MaxTriggerRetries is reached, RetryCount stays capped and retries
	// continue at MaxTriggerRetryDelay with warning logs.
	fireAt := sched.NextTime
	sched = secrettriggerschedule.AfterFire(sched, logger, "secret rotation", "secret:abc", secrets.RotateRetryDelay, fireAt)
	c.Check(sched.RetryCount, gc.Equals, secrettriggerschedule.MaxTriggerRetries)
	c.Check(sched.NextTime, gc.Equals, fireAt.Add(secrettriggerschedule.MaxTriggerRetryDelay))
	c.Check(logger.warnings, gc.HasLen, secrettriggerschedule.MaxTriggerRetries)
}

func (s *ScheduleSuite) TestSoonestTimeSkipsZero(c *gc.C) {
	now := time.Now().UTC()
	entries := map[string]secrettriggerschedule.TriggerSchedule{
		"zero":    {},
		"pending": secrettriggerschedule.NewTriggerSchedule(now.Add(2 * time.Hour)),
	}
	soonest := secrettriggerschedule.SoonestTime(entries, func(s secrettriggerschedule.TriggerSchedule) time.Time {
		return s.NextTime
	})
	c.Check(soonest, gc.Equals, now.Add(2*time.Hour))

	delete(entries, "pending")
	soonest = secrettriggerschedule.SoonestTime(entries, func(s secrettriggerschedule.TriggerSchedule) time.Time {
		return s.NextTime
	})
	c.Check(soonest.IsZero(), jc.IsTrue)
}

func (s *ScheduleSuite) TestScheduleTimer(c *gc.C) {
	clk := testclock.NewClock(time.Now())
	now := clk.Now()

	// Unarmed timer with zero trigger time is a no-op.
	timer, scheduled, delay, changed := secrettriggerschedule.ScheduleTimer(clk, nil, time.Time{}, time.Time{})
	c.Check(timer, gc.IsNil)
	c.Check(scheduled.IsZero(), jc.IsTrue)
	c.Check(delay, gc.Equals, time.Duration(0))
	c.Check(changed, jc.IsFalse)
	assertNoTimerArmed(c, clk)

	entries := map[string]time.Time{
		"a": now.Add(2 * time.Hour),
		"b": now.Add(time.Hour),
	}
	soonest := secrettriggerschedule.SoonestTime(entries, func(t time.Time) time.Time { return t })
	c.Check(soonest, gc.Equals, now.Add(time.Hour))

	timer, scheduled, delay, changed = secrettriggerschedule.ScheduleTimer(clk, nil, scheduled, soonest)
	c.Assert(timer, gc.NotNil)
	c.Check(scheduled, gc.Equals, now.Add(time.Hour))
	c.Check(delay, gc.Equals, time.Hour)
	c.Check(changed, jc.IsTrue)
	waitTimerArmed(c, clk)

	// Equal instant (even in a different location) leaves the timer unchanged.
	otherLoc := scheduled.In(time.FixedZone("UTC+2", 2*3600))
	sameTimer, sameScheduled, sameDelay, changed := secrettriggerschedule.ScheduleTimer(clk, timer, scheduled, otherLoc)
	c.Check(sameTimer, gc.Equals, timer)
	c.Check(sameScheduled, gc.Equals, scheduled)
	c.Check(sameDelay, gc.Equals, time.Duration(0))
	c.Check(changed, jc.IsFalse)
	assertNoTimerArmed(c, clk)

	// Zero current with non-nil timer (post-fire state) re-arms for the same
	// deadline.
	timer, scheduled, delay, changed = secrettriggerschedule.ScheduleTimer(clk, timer, time.Time{}, soonest)
	c.Assert(timer, gc.NotNil)
	c.Check(scheduled, gc.Equals, soonest)
	c.Check(delay, gc.Equals, time.Hour)
	c.Check(changed, jc.IsTrue)
	waitTimerArmed(c, clk)

	// Past trigger time is set to now (zero delay).
	timer, scheduled, delay, changed = secrettriggerschedule.ScheduleTimer(clk, timer, scheduled, now.Add(-time.Minute))
	c.Assert(timer, gc.NotNil)
	c.Check(scheduled, gc.Equals, now)
	c.Check(delay, gc.Equals, time.Duration(0))
	c.Check(changed, jc.IsTrue)
	waitTimerArmed(c, clk)

	// Zero trigger time stops the timer and returns nil.
	timer, scheduled, delay, changed = secrettriggerschedule.ScheduleTimer(clk, timer, scheduled, time.Time{})
	c.Check(timer, gc.IsNil)
	c.Check(scheduled.IsZero(), jc.IsTrue)
	c.Check(delay, gc.Equals, time.Duration(0))
	c.Check(changed, jc.IsTrue)
	assertNoTimerArmed(c, clk)
}

func (s *ScheduleSuite) TestScheduleAlarm(c *gc.C) {
	clk := testclock.NewClock(time.Now())
	now := clk.Now()

	// Unarmed alarm with zero trigger time is a no-op.
	alarm, scheduled, changed := secrettriggerschedule.ScheduleAlarm(clk, nil, time.Time{}, time.Time{})
	c.Check(alarm, gc.IsNil)
	c.Check(scheduled.IsZero(), jc.IsTrue)
	c.Check(changed, jc.IsFalse)
	assertNoTimerArmed(c, clk)

	deadline := now.Add(time.Hour)
	alarm, scheduled, changed = secrettriggerschedule.ScheduleAlarm(clk, nil, scheduled, deadline)
	c.Assert(alarm, gc.NotNil)
	c.Check(scheduled, gc.Equals, deadline)
	c.Check(changed, jc.IsTrue)
	waitTimerArmed(c, clk)

	// Equal instant (even in a different location) leaves the alarm unchanged.
	otherLoc := scheduled.In(time.FixedZone("UTC+2", 2*3600))
	sameAlarm, sameScheduled, changed := secrettriggerschedule.ScheduleAlarm(clk, alarm, scheduled, otherLoc)
	c.Check(sameAlarm, gc.Equals, alarm)
	c.Check(sameScheduled, gc.Equals, scheduled)
	c.Check(changed, jc.IsFalse)
	assertNoTimerArmed(c, clk)

	// Zero current with non-nil alarm (post-fire state) re-arms for the same
	// deadline.
	alarm, scheduled, changed = secrettriggerschedule.ScheduleAlarm(clk, alarm, time.Time{}, deadline)
	c.Assert(alarm, gc.NotNil)
	c.Check(scheduled, gc.Equals, deadline)
	c.Check(changed, jc.IsTrue)
	waitTimerArmed(c, clk)

	// Past trigger time is set to now.
	alarm, scheduled, changed = secrettriggerschedule.ScheduleAlarm(clk, alarm, scheduled, now.Add(-time.Minute))
	c.Assert(alarm, gc.NotNil)
	c.Check(scheduled, gc.Equals, now)
	c.Check(changed, jc.IsTrue)
	waitTimerArmed(c, clk)

	// Zero trigger time stops the alarm and returns nil.
	alarm, scheduled, changed = secrettriggerschedule.ScheduleAlarm(clk, alarm, scheduled, time.Time{})
	c.Check(alarm, gc.IsNil)
	c.Check(scheduled.IsZero(), jc.IsTrue)
	c.Check(changed, jc.IsTrue)
	assertNoTimerArmed(c, clk)
}

func waitTimerArmed(c *gc.C, clk *testclock.Clock) {
	select {
	case <-clk.Alarms():
	case <-time.After(testing.LongWait):
		c.Fatal("timed out waiting for timer to be armed")
	}
}

func assertNoTimerArmed(c *gc.C, clk *testclock.Clock) {
	select {
	case <-clk.Alarms():
		c.Fatal("unexpected timer or alarm armed")
	default:
	}
}

type fakeLogger struct {
	infos    []string
	warnings []string
}

func (l *fakeLogger) Infof(format string, args ...interface{}) {
	l.infos = append(l.infos, fmt.Sprintf(format, args...))
}

func (l *fakeLogger) Warningf(format string, args ...interface{}) {
	l.warnings = append(l.warnings, fmt.Sprintf(format, args...))
}
