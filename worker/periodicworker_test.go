// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package worker

import (
	"time"

	jtesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v2"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/testing"
)

type periodicWorkerSuite struct {
	testing.BaseSuite
}

var (
	_                   = gc.Suite(&periodicWorkerSuite{})
	defaultPeriod       = time.Second
	defaultFireOnceWait = defaultPeriod / 2
)

func (s *periodicWorkerSuite) TestWait(c *gc.C) {
	funcHasRun := make(chan struct{})
	doWork := func(_ <-chan struct{}) error {
		funcHasRun <- struct{}{}
		return testError
	}

	w := NewPeriodicWorker(doWork, defaultPeriod, NewTimer)
	defer func() { c.Assert(worker.Stop(w), gc.Equals, testError) }()
	select {
	case <-funcHasRun:
	case <-time.After(testing.ShortWait):
		c.Fatalf("The doWork function should have been called by now")
	}
	w.Kill()
	c.Assert(w.Wait(), gc.Equals, testError)
	select {
	case <-funcHasRun:
		c.Fatalf("After the kill we don't expect anymore calls to the function")
	case <-time.After(defaultFireOnceWait):
	}
}

type testNextPeriod struct {
	jtesting.Stub
}

func (t *testNextPeriod) nextPeriod(period time.Duration, jitter float64) time.Duration {
	t.MethodCall(t, "nextPeriod", period, jitter)
	return period
}

func (s *periodicWorkerSuite) TestNextPeriod(c *gc.C) {
	for i := 0; i < 100; i++ {
		p := nextPeriod(time.Second, 0.1)
		c.Assert(p.Seconds()/time.Second.Seconds() <= 1.1, jc.IsTrue)
		c.Assert(p.Seconds()/time.Second.Seconds() >= 0.9, jc.IsTrue)
	}
}

func (s *periodicWorkerSuite) TestNextPeriodWithoutJitter(c *gc.C) {
	for i := 0; i < 100; i++ {
		p := nextPeriod(time.Second, 0)
		c.Assert(p, gc.DeepEquals, time.Second)
	}
}

func (s *periodicWorkerSuite) TestWaitWithJitter(c *gc.C) {
	funcHasRun := make(chan struct{}, 1)
	doWork := func(_ <-chan struct{}) error {
		funcHasRun <- struct{}{}
		return nil
	}

	tPeriod := &testNextPeriod{}
	cleanup := jtesting.PatchValue(&nextPeriod, tPeriod.nextPeriod)
	defer cleanup()

	w := NewPeriodicWorker(doWork, testing.ShortWait, NewTimer, Jitter(0.2))
	defer func() { c.Assert(worker.Stop(w), gc.Equals, nil) }()

	select {
	case <-funcHasRun:
	case <-time.After(testing.LongWait):
		c.Fatalf("The doWork function should have been called by now")
	}

	select {
	case <-funcHasRun:
	case <-time.After(testing.LongWait):
		c.Fatalf("The doWork function should have been called by now")
	}
	w.Kill()
	c.Assert(w.Wait(), gc.Equals, nil)

	// We expect to see 2 calls to nextPeriod, corresponding to 2 calls to doWork. We then expect to see no more calls
	// to nextPeriod because we have Kill()ed the worker.
	tPeriod.CheckCalls(c, []jtesting.StubCall{{
		FuncName: "nextPeriod",
		Args:     []interface{}{testing.ShortWait, float64(0.2)},
	}, {
		FuncName: "nextPeriod",
		Args:     []interface{}{testing.ShortWait, float64(0.2)},
	}})
	select {
	case <-funcHasRun:
		c.Fatalf("After the kill we don't expect anymore calls to the function")
	case <-time.After(testing.ShortWait * 2):
		// We don't have to wait very long, just longer than timeout and Jitter would cause
	}
}

// TestWaitNil starts a periodicWorker asserts that after
// killing the worker Wait() returns nil after at least
// one call of the doWork function
func (s *periodicWorkerSuite) TestWaitNil(c *gc.C) {
	funcHasRun := make(chan struct{})
	doWork := func(_ <-chan struct{}) error {
		funcHasRun <- struct{}{}
		return nil
	}

	w := NewPeriodicWorker(doWork, defaultPeriod, NewTimer)
	defer func() { c.Assert(worker.Stop(w), gc.IsNil) }()
	select {
	case <-funcHasRun:
	case <-time.After(defaultFireOnceWait):
		c.Fatalf("The doWork function should have been called by now")
	}
	w.Kill()
	c.Assert(w.Wait(), gc.Equals, nil)
}

// TestKill starts a periodic worker and Kills it
// it expects the doWork function to be notified of this and the error from
// doWork is returned by Wait()
func (s *periodicWorkerSuite) TestKill(c *gc.C) {
	tests := []struct {
		ReturnValue   error
		ExpectedValue error
	}{
		{nil, nil},
		{testError, testError},
		{ErrKilled, nil},
	}

	for i, test := range tests {
		c.Logf("Running test %d\n", i)
		runKillTest(c, test.ReturnValue, test.ExpectedValue)
	}
}

func runKillTest(c *gc.C, returnValue, expected error) {
	ready := make(chan struct{})
	doWorkNotification := make(chan struct{})
	doWork := func(stopCh <-chan struct{}) error {
		close(ready)
		<-stopCh
		close(doWorkNotification)
		return returnValue
	}

	w := NewPeriodicWorker(doWork, defaultPeriod, NewTimer)
	defer func() { c.Assert(worker.Stop(w), gc.Equals, expected) }()

	select {
	case <-ready:
	case <-time.After(testing.LongWait):
		c.Fatalf("The doWork call should be ready by now")
	}
	w.Kill()
	select {
	case <-doWorkNotification:
	case <-time.After(testing.LongWait):
		c.Fatalf("The doWork function should have been notified of the stop by now")
	}
	c.Assert(w.Wait(), gc.Equals, expected)

	// test we can kill again without a panic and our death reason stays intact
	w.Kill()
}

// TestCallUntilKilled checks that our function is called
// at least 5 times, and that with a period of 500ms each call is made
// in a reasonable time
func (s *periodicWorkerSuite) TestCallUntilKilled(c *gc.C) {
	funcHasRun := make(chan struct{}, 5)
	doWork := func(_ <-chan struct{}) error {
		funcHasRun <- struct{}{}
		return nil
	}

	period := time.Millisecond * 500
	unacceptableWait := time.Second * 10
	w := NewPeriodicWorker(doWork, period, NewTimer)
	defer func() { c.Assert(worker.Stop(w), gc.IsNil) }()
	for i := 0; i < 5; i++ {
		select {
		case <-funcHasRun:
		case <-time.After(unacceptableWait):
			c.Fatalf("The function should have been called again by now")
		}
	}
	w.Kill()
	c.Assert(w.Wait(), gc.Equals, nil)
}
