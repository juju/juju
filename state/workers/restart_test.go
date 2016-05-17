// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package workers_test

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/clock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/state/workers"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/workertest"
)

type RestartWorkersSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&RestartWorkersSuite{})

func (*RestartWorkersSuite) TestValidateSuccess(c *gc.C) {
	config := workers.RestartConfig{
		Factory: struct{ workers.Factory }{},
		Logger:  loggo.GetLogger("test"),
		Clock:   struct{ clock.Clock }{},
		Delay:   time.Nanosecond,
	}
	err := config.Validate()
	c.Check(err, jc.ErrorIsNil)
}

func (*RestartWorkersSuite) TestValidateMissingFactory(c *gc.C) {
	config := validRestartConfig()
	config.Factory = nil
	checkInvalidRestartConfig(c, config, "nil Factory not valid")
}

func (*RestartWorkersSuite) TestValidateUninitializedLogger(c *gc.C) {
	config := validRestartConfig()
	config.Logger = loggo.Logger{}
	checkInvalidRestartConfig(c, config, "uninitialized Logger not valid")
}

func (*RestartWorkersSuite) TestValidateMissingClock(c *gc.C) {
	config := validRestartConfig()
	config.Clock = nil
	checkInvalidRestartConfig(c, config, "nil Clock not valid")
}

func (*RestartWorkersSuite) TestValidateMissingDelay(c *gc.C) {
	config := validRestartConfig()
	config.Delay = 0
	checkInvalidRestartConfig(c, config, "non-positive Delay not valid")
}

func (*RestartWorkersSuite) TestValidateNegativeDelay(c *gc.C) {
	config := validRestartConfig()
	config.Delay = -time.Second
	checkInvalidRestartConfig(c, config, "non-positive Delay not valid")
}

func (*RestartWorkersSuite) TestNewLeadershipManagerError(c *gc.C) {
	fix := Fixture{
		LW_errors: []error{ErrFailStart},
	}
	fix.FailRestart(c, "cannot create leadership lease manager: bad start")
}

func (*RestartWorkersSuite) TestNewSingularManagerError(c *gc.C) {
	fix := Fixture{
		LW_errors: []error{nil},
		SW_errors: []error{ErrFailStart},
	}
	fix.FailRestart(c, "cannot create singular lease manager: bad start")
}

func (*RestartWorkersSuite) TestNewTxnLogWatcherError(c *gc.C) {
	fix := Fixture{
		LW_errors:  []error{nil},
		SW_errors:  []error{nil},
		TLW_errors: []error{ErrFailStart},
	}
	fix.FailRestart(c, "cannot create transaction log watcher: bad start")
}

func (*RestartWorkersSuite) TestNewPresenceWatcherError(c *gc.C) {
	fix := BasicFixture()
	fix.PW_errors = []error{ErrFailStart}
	fix.FailRestart(c, "cannot create presence watcher: bad start")
}

func (*RestartWorkersSuite) TestLeadershipManagerDelay(c *gc.C) {
	fix := BasicFixture()
	fix.LW_errors = []error{errors.New("oof")}
	fix.RunRestart(c, func(ctx Context, rw *workers.RestartWorkers) {
		w := NextWorker(c, ctx.LWs())
		c.Assert(w, gc.NotNil)
		AssertWorker(c, rw.LeadershipManager(), w)
		w.Kill()

		clock := ctx.Clock()
		WaitAlarms(c, clock, 1)
		clock.Advance(almostFiveSeconds)
		AssertWorker(c, rw.LeadershipManager(), w)

		err := workertest.CheckKill(c, rw)
		c.Check(err, gc.ErrorMatches, "error stopping leadership lease manager: oof")
	})
}

func (*RestartWorkersSuite) TestSingularManagerDelay(c *gc.C) {
	fix := BasicFixture()
	fix.SW_errors = []error{errors.New("oof")}
	fix.RunRestart(c, func(ctx Context, rw *workers.RestartWorkers) {
		w := NextWorker(c, ctx.SWs())
		c.Assert(w, gc.NotNil)
		AssertWorker(c, rw.SingularManager(), w)
		w.Kill()

		clock := ctx.Clock()
		WaitAlarms(c, clock, 1)
		clock.Advance(almostFiveSeconds)
		AssertWorker(c, rw.SingularManager(), w)

		err := workertest.CheckKill(c, rw)
		c.Check(err, gc.ErrorMatches, "error stopping singular lease manager: oof")
	})
}

func (*RestartWorkersSuite) TestTxnLogWatcherDelay(c *gc.C) {
	fix := BasicFixture()
	fix.TLW_errors = []error{errors.New("oof")}
	fix.RunRestart(c, func(ctx Context, rw *workers.RestartWorkers) {
		w := NextWorker(c, ctx.TLWs())
		c.Assert(w, gc.NotNil)
		AssertWorker(c, rw.TxnLogWatcher(), w)
		w.Kill()

		clock := ctx.Clock()
		WaitAlarms(c, clock, 1)
		clock.Advance(almostFiveSeconds)
		AssertWorker(c, rw.TxnLogWatcher(), w)

		err := workertest.CheckKill(c, rw)
		c.Check(err, gc.ErrorMatches, "error stopping transaction log watcher: oof")
	})
}

func (*RestartWorkersSuite) TestPresenceWatcherDelay(c *gc.C) {
	fix := BasicFixture()
	fix.PW_errors = []error{errors.New("oof")}
	fix.RunRestart(c, func(ctx Context, rw *workers.RestartWorkers) {
		w := NextWorker(c, ctx.PWs())
		c.Assert(w, gc.NotNil)
		AssertWorker(c, rw.PresenceWatcher(), w)
		w.Kill()

		clock := ctx.Clock()
		WaitAlarms(c, clock, 1)
		clock.Advance(almostFiveSeconds)
		AssertWorker(c, rw.PresenceWatcher(), w)

		err := workertest.CheckKill(c, rw)
		c.Check(err, gc.ErrorMatches, "error stopping presence watcher: oof")
	})
}

func (*RestartWorkersSuite) TestLeadershipManagerRestart(c *gc.C) {
	fix := BasicFixture()
	fix.LW_errors = []error{errors.New("oof"), nil}
	fix.RunRestart(c, func(ctx Context, rw *workers.RestartWorkers) {
		w := NextWorker(c, ctx.LWs())
		c.Assert(w, gc.NotNil)
		AssertWorker(c, rw.LeadershipManager(), w)
		w.Kill()

		clock := ctx.Clock()
		WaitAlarms(c, clock, 1)
		clock.Advance(fiveSeconds)
		w2 := NextWorker(c, ctx.LWs())
		c.Assert(w, gc.NotNil)
		WaitWorker(c, LM_getter(rw), w2)

		workertest.CleanKill(c, rw)
	})
}

func (*RestartWorkersSuite) TestSingularManagerRestart(c *gc.C) {
	fix := BasicFixture()
	fix.SW_errors = []error{errors.New("oof"), nil}
	fix.RunRestart(c, func(ctx Context, rw *workers.RestartWorkers) {
		w := NextWorker(c, ctx.SWs())
		c.Assert(w, gc.NotNil)
		AssertWorker(c, rw.SingularManager(), w)
		w.Kill()

		clock := ctx.Clock()
		WaitAlarms(c, clock, 1)
		clock.Advance(fiveSeconds)
		w2 := NextWorker(c, ctx.SWs())
		c.Assert(w, gc.NotNil)
		WaitWorker(c, SM_getter(rw), w2)

		workertest.CleanKill(c, rw)
	})
}

func (*RestartWorkersSuite) TestTxnLogWatcherRestart(c *gc.C) {
	fix := BasicFixture()
	fix.TLW_errors = []error{errors.New("oof"), nil}
	fix.RunRestart(c, func(ctx Context, rw *workers.RestartWorkers) {
		w := NextWorker(c, ctx.TLWs())
		c.Assert(w, gc.NotNil)
		AssertWorker(c, rw.TxnLogWatcher(), w)
		w.Kill()

		clock := ctx.Clock()
		WaitAlarms(c, clock, 1)
		clock.Advance(fiveSeconds)
		w2 := NextWorker(c, ctx.TLWs())
		c.Assert(w, gc.NotNil)
		WaitWorker(c, TLW_getter(rw), w2)

		workertest.CleanKill(c, rw)
	})
}

func (*RestartWorkersSuite) TestPresenceWatcherRestart(c *gc.C) {
	fix := BasicFixture()
	fix.PW_errors = []error{errors.New("oof"), nil}
	fix.RunRestart(c, func(ctx Context, rw *workers.RestartWorkers) {
		w := NextWorker(c, ctx.PWs())
		c.Assert(w, gc.NotNil)
		AssertWorker(c, rw.PresenceWatcher(), w)
		w.Kill()

		clock := ctx.Clock()
		WaitAlarms(c, clock, 1)
		clock.Advance(fiveSeconds)
		w2 := NextWorker(c, ctx.PWs())
		c.Assert(w, gc.NotNil)
		WaitWorker(c, PW_getter(rw), w2)

		workertest.CleanKill(c, rw)
	})
}

func (*RestartWorkersSuite) TestStopsAllWorkers(c *gc.C) {
	fix := BasicFixture()
	fix.RunRestart(c, func(ctx Context, rw *workers.RestartWorkers) {
		workertest.CleanKill(c, rw)
		for _, ch := range []<-chan worker.Worker{
			ctx.LWs(), ctx.SWs(), ctx.TLWs(), ctx.PWs(),
		} {
			w := NextWorker(c, ch)
			workertest.CheckKilled(c, w)
		}
	})
}

func validRestartConfig() workers.RestartConfig {
	return workers.RestartConfig{
		Factory: struct{ workers.Factory }{},
		Logger:  loggo.GetLogger("test"),
		Clock:   struct{ clock.Clock }{},
		Delay:   time.Nanosecond,
	}
}

func checkInvalidRestartConfig(c *gc.C, config workers.RestartConfig, match string) {
	check := func(err error) {
		c.Check(err, jc.Satisfies, errors.IsNotValid)
		c.Check(err, gc.ErrorMatches, match)
	}

	err := config.Validate()
	check(err)

	rw, err := workers.NewRestartWorkers(config)
	if !c.Check(rw, gc.IsNil) {
		workertest.DirtyKill(c, rw)
	}
	check(err)
}
