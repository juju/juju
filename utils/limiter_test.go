// Copyright 2011, 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package utils_test

import (
	"fmt"
	"time"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/utils"
)

const longWait = 10 * time.Second

type limiterSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&limiterSuite{})

func (*limiterSuite) TestAcquireUntilFull(c *gc.C) {
	l := utils.NewLimiter(2)
	c.Check(l.Acquire(), jc.IsTrue)
	c.Check(l.Acquire(), jc.IsTrue)
	c.Check(l.Acquire(), jc.IsFalse)
}

func (*limiterSuite) TestBadRelease(c *gc.C) {
	l := utils.NewLimiter(2)
	c.Check(l.Release(), gc.ErrorMatches, "Release without an associated Acquire")
}

func (*limiterSuite) TestAcquireAndRelease(c *gc.C) {
	l := utils.NewLimiter(2)
	c.Check(l.Acquire(), jc.IsTrue)
	c.Check(l.Acquire(), jc.IsTrue)
	c.Check(l.Acquire(), jc.IsFalse)
	c.Check(l.Release(), gc.IsNil)
	c.Check(l.Acquire(), jc.IsTrue)
	c.Check(l.Release(), gc.IsNil)
	c.Check(l.Release(), gc.IsNil)
	c.Check(l.Release(), gc.ErrorMatches, "Release without an associated Acquire")
}

func (*limiterSuite) TestAcquireWaitBlocksUntilRelease(c *gc.C) {
	l := utils.NewLimiter(2)
	calls := make([]string, 0, 10)
	start := make(chan bool, 0)
	waiting := make(chan bool, 0)
	done := make(chan bool, 0)
	go func() {
		<-start
		calls = append(calls, fmt.Sprintf("%v", l.Acquire()))
		calls = append(calls, fmt.Sprintf("%v", l.Acquire()))
		calls = append(calls, fmt.Sprintf("%v", l.Acquire()))
		waiting <- true
		l.AcquireWait()
		calls = append(calls, "waited")
		calls = append(calls, fmt.Sprintf("%v", l.Acquire()))
		done <- true
	}()
	// Start the routine, and wait for it to get to the first checkpoint
	start <- true
	select {
	case <-waiting:
	case <-time.After(longWait):
		c.Fatalf("timed out waiting for 'waiting' to trigger")
	}
	c.Check(l.Acquire(), jc.IsFalse)
	l.Release()
	select {
	case <-done:
	case <-time.After(longWait):
		c.Fatalf("timed out waiting for 'done' to trigger")
	}
	c.Check(calls, gc.DeepEquals, []string{"true", "true", "false", "waited", "false"})
}
