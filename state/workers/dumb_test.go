// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package workers_test

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/state/workers"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/workertest"
)

type DumbWorkersSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&DumbWorkersSuite{})

func (*DumbWorkersSuite) TestValidateSuccess(c *gc.C) {
	config := workers.DumbConfig{
		Factory: struct{ workers.Factory }{},
		Logger:  loggo.GetLogger("test"),
	}
	err := config.Validate()
	c.Check(err, jc.ErrorIsNil)
}

func (*DumbWorkersSuite) TestValidateUninitializedLogger(c *gc.C) {
	config := workers.DumbConfig{
		Factory: struct{ workers.Factory }{},
	}
	check := func(err error) {
		c.Check(err, gc.ErrorMatches, "uninitialized Logger not valid")
		c.Check(err, jc.Satisfies, errors.IsNotValid)
	}

	check(config.Validate())
	w, err := workers.NewDumbWorkers(config)
	if !c.Check(w, gc.IsNil) {
		workertest.CleanKill(c, w)
	}
	check(err)
}

func (*DumbWorkersSuite) TestValidateMissingFactory(c *gc.C) {
	config := workers.DumbConfig{
		Logger: loggo.GetLogger("test"),
	}
	check := func(err error) {
		c.Check(err, gc.ErrorMatches, "nil Factory not valid")
		c.Check(err, jc.Satisfies, errors.IsNotValid)
	}

	check(config.Validate())
	w, err := workers.NewDumbWorkers(config)
	if !c.Check(w, gc.IsNil) {
		workertest.CleanKill(c, w)
	}
	check(err)
}

func (*DumbWorkersSuite) TestNewLeadershipManagerError(c *gc.C) {
	fix := Fixture{
		LW_errors: []error{ErrFailStart},
	}
	fix.FailDumb(c, "cannot create leadership lease manager: bad start")
}

func (*DumbWorkersSuite) TestNewSingularManagerError(c *gc.C) {
	fix := Fixture{
		LW_errors: []error{nil},
		SW_errors: []error{ErrFailStart},
	}
	fix.FailDumb(c, "cannot create singular lease manager: bad start")
}

func (*DumbWorkersSuite) TestNewTxnLogWatcherError(c *gc.C) {
	fix := Fixture{
		LW_errors:  []error{nil},
		SW_errors:  []error{nil},
		TLW_errors: []error{ErrFailStart},
	}
	fix.FailDumb(c, "cannot create transaction log watcher: bad start")
}

func (*DumbWorkersSuite) TestNewPresenceWatcherError(c *gc.C) {
	fix := BasicFixture()
	fix.PW_errors = []error{ErrFailStart}
	fix.FailDumb(c, "cannot create presence watcher: bad start")
}

func (*DumbWorkersSuite) TestLeadershipManagerFails(c *gc.C) {
	fix := BasicFixture()
	fix.LW_errors = []error{errors.New("zap")}
	fix.RunDumb(c, func(ctx Context, dw *workers.DumbWorkers) {
		w := NextWorker(c, ctx.LWs())
		c.Assert(w, gc.NotNil)
		WaitWorker(c, dw.LeadershipManager(), w)

		w.Kill()
		workertest.CheckAlive(c, dw)
		WaitWorker(c, dw.LeadershipManager(), w)

		err := workertest.CheckKill(c, dw)
		c.Check(err, gc.ErrorMatches, "error stopping leadership lease manager: zap")
	})
}

func (*DumbWorkersSuite) TestSingularManagerFails(c *gc.C) {
	fix := BasicFixture()
	fix.SW_errors = []error{errors.New("zap")}
	fix.RunDumb(c, func(ctx Context, dw *workers.DumbWorkers) {
		w := NextWorker(c, ctx.SWs())
		c.Assert(w, gc.NotNil)
		WaitWorker(c, dw.SingularManager(), w)

		w.Kill()
		workertest.CheckAlive(c, dw)
		WaitWorker(c, dw.SingularManager(), w)

		err := workertest.CheckKill(c, dw)
		c.Check(err, gc.ErrorMatches, "error stopping singular lease manager: zap")
	})
}

func (*DumbWorkersSuite) TestTxnLogWatcherFails(c *gc.C) {
	fix := BasicFixture()
	fix.TLW_errors = []error{errors.New("zap")}
	fix.RunDumb(c, func(ctx Context, dw *workers.DumbWorkers) {
		w := NextWorker(c, ctx.TLWs())
		c.Assert(w, gc.NotNil)
		WaitWorker(c, dw.TxnLogWatcher(), w)

		w.Kill()
		workertest.CheckAlive(c, dw)
		WaitWorker(c, dw.TxnLogWatcher(), w)

		err := workertest.CheckKill(c, dw)
		c.Check(err, gc.ErrorMatches, "error stopping transaction log watcher: zap")
	})
}

func (*DumbWorkersSuite) TestPresenceWatcherFails(c *gc.C) {
	fix := BasicFixture()
	fix.PW_errors = []error{errors.New("zap")}
	fix.RunDumb(c, func(ctx Context, dw *workers.DumbWorkers) {
		w := NextWorker(c, ctx.PWs())
		c.Assert(w, gc.NotNil)
		WaitWorker(c, dw.PresenceWatcher(), w)

		w.Kill()
		workertest.CheckAlive(c, dw)
		WaitWorker(c, dw.PresenceWatcher(), w)

		err := workertest.CheckKill(c, dw)
		c.Check(err, gc.ErrorMatches, "error stopping presence watcher: zap")
	})
}

func (*DumbWorkersSuite) TestEverythingFails(c *gc.C) {
	fix := Fixture{
		LW_errors:  []error{errors.New("zot")},
		SW_errors:  []error{errors.New("bif")},
		TLW_errors: []error{errors.New("pow")},
		PW_errors:  []error{errors.New("arg")},
	}
	fix.RunDumb(c, func(ctx Context, dw *workers.DumbWorkers) {
		for _, ch := range []<-chan worker.Worker{
			ctx.LWs(), ctx.SWs(), ctx.TLWs(), ctx.PWs(),
		} {
			w := NextWorker(c, ch)
			c.Assert(w, gc.NotNil)
			w.Kill()
		}

		workertest.CheckAlive(c, dw)
		err := workertest.CheckKill(c, dw)
		c.Check(err, gc.ErrorMatches, "error stopping transaction log watcher: pow")
	})
}

func (*DumbWorkersSuite) TestStopsAllWorkers(c *gc.C) {
	fix := BasicFixture()
	fix.RunDumb(c, func(ctx Context, dw *workers.DumbWorkers) {
		workertest.CleanKill(c, dw)
		for _, ch := range []<-chan worker.Worker{
			ctx.LWs(), ctx.SWs(), ctx.TLWs(), ctx.PWs(),
		} {
			w := NextWorker(c, ch)
			workertest.CheckKilled(c, w)
		}
	})
}
