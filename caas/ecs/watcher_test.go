// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ecs_test

import (
	"time"

	"github.com/juju/clock/testclock"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v2/workertest"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/caas/ecs"
	"github.com/juju/juju/testing"
)

type watcherSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&watcherSuite{})

func (s *watcherSuite) TestWatcher(c *gc.C) {
	clock := testclock.NewClock(time.Time{})

	checkerResultChan := make(chan bool)
	checker := func() (bool, error) {
		select {
		case ok := <-checkerResultChan:
			return ok, nil
		}
	}

	w, err := ecs.NewNotifyWatcher("test-watcher", clock, checker)
	c.Assert(err, jc.ErrorIsNil)

	assertNotified := func() {
		select {
		case <-w.Changes():
		case <-time.After(testing.LongWait):
			c.Fatalf("timed out waiting for changes notified")
		}
	}

	assertNotNotified := func() {
		select {
		case <-w.Changes():
			c.Fatalf("unexpected change notified")
		case <-time.After(testing.ShortWait):
			return
		}
	}

	// consume initial notification.
	assertNotified()

	err = clock.WaitAdvance(1*time.Second, testing.ShortWait, 2)
	c.Assert(err, jc.ErrorIsNil)
	select {
	case checkerResultChan <- true:
		assertNotified()
	case <-time.After(testing.LongWait):
		c.Fatalf("timed out waiting for checker result passed through")
	}

	err = clock.WaitAdvance(1*time.Second, testing.ShortWait, 2)
	c.Assert(err, jc.ErrorIsNil)
	select {
	case checkerResultChan <- true:
		assertNotified()
	case <-time.After(testing.LongWait):
		c.Fatalf("timed out waiting for checker result passed through")
	}

	err = clock.WaitAdvance(1*time.Second, testing.ShortWait, 2)
	c.Assert(err, jc.ErrorIsNil)
	select {
	case checkerResultChan <- false:
		assertNotNotified()
	case <-time.After(testing.LongWait):
		c.Fatalf("timed out waiting for checker result passed through")
	}

	w.Kill()
	c.Assert(workertest.CheckKilled(c, w), jc.ErrorIsNil)
}
