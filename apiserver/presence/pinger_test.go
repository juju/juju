// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package presence_test

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/presence"
	"github.com/juju/juju/worker/workertest"
)

type WorkerSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&WorkerSuite{})

func (s *WorkerSuite) TestConfigValidateOkay(c *gc.C) {
	err := validConfig().Validate()
	c.Check(err, jc.ErrorIsNil)
}

func (s *WorkerSuite) TestConfigValidateMissingIdentity(c *gc.C) {
	config := validConfig()
	config.Identity = nil
	checkInvalid(c, config, "nil Identity not valid")
}

func (s *WorkerSuite) TestConfigValidateMissingStart(c *gc.C) {
	config := validConfig()
	config.Start = nil
	checkInvalid(c, config, "nil Start not valid")
}

func (s *WorkerSuite) TestConfigValidateMissingClock(c *gc.C) {
	config := validConfig()
	config.Clock = nil
	checkInvalid(c, config, "nil Clock not valid")
}

func (s *WorkerSuite) TestConfigValidateMissingRetryDelay(c *gc.C) {
	config := validConfig()
	config.RetryDelay = 0
	checkInvalid(c, config, `non-positive RetryDelay not valid`)
}

func (s *WorkerSuite) TestConfigValidateNegativeRetryDelay(c *gc.C) {
	config := validConfig()
	config.RetryDelay = -time.Minute
	checkInvalid(c, config, `non-positive RetryDelay not valid`)
}

func (s *WorkerSuite) TestInitialSuccess(c *gc.C) {
	fix := NewFixture()
	stub := fix.Run(c, func(context Context, worker *presence.Worker) {
		workertest.CleanKill(c, worker)
		// Despite immediate kill, a pinger was still started.
		context.WaitPinger()
	})
	stub.CheckCallNames(c, "Start")
}

func (s *WorkerSuite) TestInitialFailedStart(c *gc.C) {
	// First start attempt fails.
	fix := NewFixture(errors.New("zap"))
	stub := fix.Run(c, func(context Context, worker *presence.Worker) {
		workertest.CleanKill(c, worker)
		// Despite immediate kill, we didn't exit until the
		// second time through the loop.
		context.WaitAlarms(2)
	})
	stub.CheckCallNames(c, "Start")
}

func (s *WorkerSuite) TestInitialRetryIsDelayed(c *gc.C) {
	// First start attempt fails.
	fix := NewFixture(errors.New("zap"))
	stub := fix.Run(c, func(context Context, worker *presence.Worker) {
		context.WaitAlarms(2)
		// Now we know the worker is waiting to start the next
		// pinger, advance *almost* far enough to trigger it.
		context.AdvanceClock(almostFiveSeconds)
		workertest.CheckAlive(c, worker)
	})
	stub.CheckCallNames(c, "Start")
}

func (s *WorkerSuite) TestInitialRetryIsNotDelayedTooMuch(c *gc.C) {
	// First start attempt fails.
	fix := NewFixture(errors.New("zap"))
	stub := fix.Run(c, func(context Context, worker *presence.Worker) {
		context.WaitAlarms(2)
		// Now we know the worker is waiting to start the next
		// pinger, advance *just* far enough to trigger it.
		context.AdvanceClock(fiveSeconds)
		context.WaitPinger()
	})
	stub.CheckCallNames(c, "Start", "Start")
}

func (s *WorkerSuite) TestFailedPingerRestartIsDelayed(c *gc.C) {
	// First start succeeds; pinger will die with error when killed.
	fix := NewFixture(nil, errors.New("zap"))
	stub := fix.Run(c, func(context Context, worker *presence.Worker) {
		context.WaitPinger().Kill()
		context.WaitAlarms(2)
		// Now we know the first pinger has been stopped, and
		// the worker is waiting to start the next one, advance
		// *almost* far enough to trigger it.
		context.AdvanceClock(almostFiveSeconds)
		workertest.CheckAlive(c, worker)
	})
	stub.CheckCallNames(c, "Start")
}

func (s *WorkerSuite) TestFailedPingerRestartIsNotDelayedTooMuch(c *gc.C) {
	// First start succeeds; pinger will die with error when killed.
	fix := NewFixture(nil, errors.New("zap"))
	stub := fix.Run(c, func(context Context, worker *presence.Worker) {
		context.WaitPinger().Kill()
		context.WaitAlarms(2)
		// Now we know the first pinger has been stopped, and
		// the worker is waiting to start the next one, advance
		// *just* far enough to trigger it.
		context.AdvanceClock(fiveSeconds)
		context.WaitPinger()
	})
	stub.CheckCallNames(c, "Start", "Start")
}

func (s *WorkerSuite) TestStoppedPingerRestartIsDelayed(c *gc.C) {
	fix := NewFixture()
	stub := fix.Run(c, func(context Context, worker *presence.Worker) {
		context.WaitPinger().Kill()
		context.WaitAlarms(2)
		// Now we know the first pinger has been stopped (no
		// error), and the worker is waiting to start the next
		// one, advance *almost* far enough to trigger it.
		context.AdvanceClock(almostFiveSeconds)
		workertest.CheckAlive(c, worker)
	})
	stub.CheckCallNames(c, "Start")
}

func (s *WorkerSuite) TestStoppedPingerRestartIsNotDelayedTooMuch(c *gc.C) {
	fix := NewFixture()
	stub := fix.Run(c, func(context Context, worker *presence.Worker) {
		context.WaitPinger().Kill()
		context.WaitAlarms(2)
		// Now we know the first pinger has been stopped (no
		// error), and the worker is waiting to start the next
		// one, advance *just* far enough to trigger it.
		context.AdvanceClock(fiveSeconds)
		context.WaitPinger()
	})
	stub.CheckCallNames(c, "Start", "Start")
}

func (s *WorkerSuite) TestManyRestarts(c *gc.C) {
	fix := NewFixture()
	stub := fix.Run(c, func(context Context, worker *presence.Worker) {
		context.WaitAlarms(1)
		for i := 0; i < 4; i++ {
			context.WaitPinger().Kill()
			context.WaitAlarms(1)
			context.AdvanceClock(fiveSeconds)
		}
		workertest.CheckAlive(c, worker)
	})
	stub.CheckCallNames(c, "Start", "Start", "Start", "Start", "Start")
}
