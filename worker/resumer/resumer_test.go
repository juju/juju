// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resumer_test

import (
	"errors"
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v2/workertest"
	gc "gopkg.in/check.v1"

	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/resumer"
)

type ResumerSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&ResumerSuite{})

func (*ResumerSuite) TestImmediateFailure(c *gc.C) {
	fix := newFixture(errors.New("zap"))
	stub := fix.Run(c, func(_ *testclock.Clock, worker *resumer.Resumer) {
		err := workertest.CheckKilled(c, worker)
		c.Check(err, gc.ErrorMatches, "cannot resume transactions: zap")
	})
	stub.CheckCallNames(c, "ResumeTransactions")
}

func (*ResumerSuite) TestWaitsToResume(c *gc.C) {
	fix := newFixture(nil, errors.New("unexpected"))
	stub := fix.Run(c, func(clock *testclock.Clock, worker *resumer.Resumer) {
		waitAlarms(c, clock, 2)
		clock.Advance(time.Hour - time.Nanosecond)
		workertest.CheckAlive(c, worker)
		workertest.CleanKill(c, worker)
	})
	stub.CheckCallNames(c, "ResumeTransactions")
}

func (*ResumerSuite) TestResumesAfterWait(c *gc.C) {
	fix := newFixture(nil, nil, errors.New("unexpected"))
	stub := fix.Run(c, func(clock *testclock.Clock, worker *resumer.Resumer) {
		waitAlarms(c, clock, 2)
		clock.Advance(time.Hour)
		waitAlarms(c, clock, 1)
		workertest.CleanKill(c, worker)
	})
	stub.CheckCallNames(c, "ResumeTransactions", "ResumeTransactions")
}

func (*ResumerSuite) TestSeveralResumes(c *gc.C) {
	fix := newFixture(nil, nil, nil, errors.New("unexpected"))
	stub := fix.Run(c, func(clock *testclock.Clock, worker *resumer.Resumer) {
		waitAlarms(c, clock, 2)
		clock.Advance(time.Hour)
		waitAlarms(c, clock, 1)
		clock.Advance(time.Hour)
		waitAlarms(c, clock, 1)
		workertest.CleanKill(c, worker)
	})
	stub.CheckCallNames(c, "ResumeTransactions", "ResumeTransactions", "ResumeTransactions")
}

func newFixture(errs ...error) *fixture {
	return &fixture{errors: errs}
}

type fixture struct {
	errors []error
}

type TestFunc func(*testclock.Clock, *resumer.Resumer)

func (fix fixture) Run(c *gc.C, test TestFunc) *testing.Stub {

	stub := &testing.Stub{}
	stub.SetErrors(fix.errors...)
	clock := testclock.NewClock(time.Now())
	facade := newMockFacade(stub)

	worker, err := resumer.NewResumer(resumer.Config{
		Facade:   facade,
		Interval: time.Hour,
		Clock:    clock,
	})
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.DirtyKill(c, worker)

	test(clock, worker)
	return stub
}

func newMockFacade(stub *testing.Stub) *mockFacade {
	return &mockFacade{stub: stub}
}

type mockFacade struct {
	stub *testing.Stub
}

func (mock *mockFacade) ResumeTransactions() error {
	mock.stub.AddCall("ResumeTransactions")
	return mock.stub.NextErr()
}

func waitAlarms(c *gc.C, clock *testclock.Clock, count int) {
	timeout := time.After(coretesting.LongWait)
	for i := 0; i < count; i++ {
		select {
		case <-clock.Alarms():
		case <-timeout:
			c.Fatalf("timed out waiting for alarm %d", i)
		}
	}
}
