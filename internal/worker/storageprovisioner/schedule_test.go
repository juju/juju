// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storageprovisioner

import (
	"testing"
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/tc"

	"github.com/juju/juju/internal/worker/storageprovisioner/internal/schedule"
)

type scheduleOperationsSuite struct{}

func TestScheduleOperationsSuite(t *testing.T) {
	tc.Run(t, &scheduleOperationsSuite{})
}

func (*scheduleOperationsSuite) TestSkipsDuplicateOperation(c *tc.C) {
	clock := testclock.NewClock(time.Time{})
	deps := dependencies{
		config:   Config{Clock: clock},
		schedule: schedule.NewSchedule(clock),
	}
	first := &testScheduleOp{keyValue: "key", delayValue: time.Minute}
	duplicate := &testScheduleOp{keyValue: "key"}

	scheduleOperations(&deps, first, duplicate)

	c.Check(first.delayCalls, tc.Equals, 1)
	c.Check(duplicate.delayCalls, tc.Equals, 0)
	c.Check(deps.schedule.Ready(clock.Now()), tc.HasLen, 0)
	clock.Advance(time.Minute)
	c.Check(deps.schedule.Ready(clock.Now()), tc.DeepEquals, []any{first})
}

func (*scheduleOperationsSuite) TestDuplicatePreservesPendingRetry(c *tc.C) {
	clock := testclock.NewClock(time.Time{})
	deps := dependencies{
		config:   Config{Clock: clock},
		schedule: schedule.NewSchedule(clock),
	}
	retry := &testBackoffScheduleOp{keyValue: "key"}

	scheduleOperations(&deps, retry)
	c.Assert(deps.schedule.Ready(clock.Now()), tc.DeepEquals, []any{retry})
	scheduleOperations(&deps, retry)

	duplicate := &testBackoffScheduleOp{keyValue: "key"}
	scheduleOperations(&deps, duplicate)

	c.Check(duplicate.d, tc.Equals, time.Duration(0))
	c.Check(deps.schedule.Ready(clock.Now()), tc.HasLen, 0)
	clock.Advance(minRetryDelay)
	c.Check(deps.schedule.Ready(clock.Now()), tc.DeepEquals, []any{retry})
}

type testScheduleOp struct {
	keyValue   any
	delayValue time.Duration
	delayCalls int
}

func (o *testScheduleOp) key() any {
	return o.keyValue
}

func (o *testScheduleOp) delay() time.Duration {
	o.delayCalls++
	return o.delayValue
}

type testBackoffScheduleOp struct {
	exponentialBackoff
	keyValue any
}

func (o *testBackoffScheduleOp) key() any {
	return o.keyValue
}
