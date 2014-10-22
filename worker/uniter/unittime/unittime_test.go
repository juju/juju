// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package unittime_test

import (
	stdtesting "testing"
	"time"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/worker/uniter/unittime"
)

type unitTimeSuite struct {
	currentTime time.Time
}

func Test(t *stdtesting.T) {
	gc.TestingT(t)
}

var _ = gc.Suite(&unitTimeSuite{})

func (u *unitTimeSuite) startTime() {
	u.currentTime = time.Time{}
	unittime.PatchTime(u.currentTime)
}

func (u *unitTimeSuite) forward(seconds int) {
	u.currentTime = u.currentTime.Add(time.Duration(seconds) * time.Second)
	unittime.PatchTime(u.currentTime)
}

func (u *unitTimeSuite) TestStart(c *gc.C) {
	counter := unittime.UnitTimeCounter{}
	u.startTime()
	counter.Start()
	u.forward(1)
	c.Assert(counter.Value(), gc.Equals, time.Second)
}

func (u *unitTimeSuite) TestPause(c *gc.C) {
	counter := unittime.UnitTimeCounter{}
	u.startTime()
	counter.Start()
	u.forward(1)
	counter.Stop()
	u.forward(2)
	c.Assert(counter.Value(), gc.Equals, time.Second)
}

func (u *unitTimeSuite) TestPauseStart(c *gc.C) {
	counter := unittime.UnitTimeCounter{}
	u.startTime()
	counter.Start()
	u.forward(1)
	counter.Stop()
	u.forward(1)
	counter.Start()
	u.forward(1)
	c.Assert(counter.Value(), gc.Equals, 2*time.Second)
}

func (u *unitTimeSuite) TestPauseStartSleep(c *gc.C) {
	counter := unittime.UnitTimeCounter{}
	u.startTime()
	counter.Start()
	u.forward(1)
	counter.Stop()
	u.forward(1)
	counter.Start()
	u.forward(1)
	c.Assert(counter.Value(), gc.Equals, 2*time.Second)
}

func (u *unitTimeSuite) TestRunning(c *gc.C) {
	counter := unittime.UnitTimeCounter{}
	c.Assert(counter.Running(), jc.IsFalse)
	counter.Start()
	c.Assert(counter.Running(), jc.IsTrue)
	counter.Stop()
	c.Assert(counter.Running(), jc.IsFalse)
}

func (u *unitTimeSuite) TestConsecutiveStart(c *gc.C) {
	counter := unittime.UnitTimeCounter{}
	u.startTime()
	counter.Start()
	u.forward(2)
	counter.Start()
	u.forward(1)
	c.Assert(counter.Value(), gc.Equals, 3*time.Second)
}
