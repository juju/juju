// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package worker

import (
	"time"

	gc "launchpad.net/gocheck"

	"github.com/juju/juju/testing"
)

type periodicWorkerSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&periodicWorkerSuite{})

func (s *periodicWorkerSuite) TestWait(c *gc.C) {
	funcHasRun := make(chan struct{})
	doWork := func(_ <-chan struct{}) error {
		funcHasRun <- struct{}{}
		return testError
	}

	w := NewPeriodicWorker(doWork, time.Second)
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
	case <-time.After(testing.ShortWait):
	}
}

func (s *periodicWorkerSuite) TestWaitNil(c *gc.C) {
	funcHasRun := make(chan struct{})
	doWork := func(_ <-chan struct{}) error {
		funcHasRun <- struct{}{}
		return nil
	}

	w := NewPeriodicWorker(doWork, time.Second)
	select {
	case <-funcHasRun:
	case <-time.After(testing.ShortWait):
		c.Fatalf("The doWork function should have been called by now")
	}
	w.Kill()
	c.Assert(w.Wait(), gc.Equals, nil)
}

func (s *periodicWorkerSuite) TestKill(c *gc.C) {
	doWork := func(stopCh <-chan struct{}) error {
		<-stopCh
		return testError
	}

	w := NewPeriodicWorker(doWork, time.Second)
	w.Kill()
	c.Assert(w.Wait(), gc.Equals, testError)

	// test we can kill again without a panic
	w.Kill()
}

// TestCallUntilKilled checks that our function is called
// at least 5 times, and that with a period of 1ms each call is made
// in a reasonable time
func (s *periodicWorkerSuite) TestCallUntilKilled(c *gc.C) {
	funcHasRun := make(chan struct{})
	doWork := func(_ <-chan struct{}) error {
		funcHasRun <- struct{}{}
		return nil
	}

	w := NewPeriodicWorker(doWork, time.Millisecond)
	for i := 0; i < 5; i++ {
		select {
		case <-funcHasRun:
			continue
		case <-time.Tick(time.Second):
			c.Fatalf("The function should have been called again by now")
		}
	}
	w.Kill()
	c.Assert(w.Wait(), gc.Equals, nil)
}
