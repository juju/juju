// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package worker_test

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v2"
	"github.com/juju/worker/v2/workertest"
	gc "gopkg.in/check.v1"

	coretesting "github.com/juju/juju/testing"
)

type WorkerSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&WorkerSuite{})

func (*WorkerSuite) TestStopReturnsNoError(c *gc.C) {
	w := workertest.NewDeadWorker(nil)

	err := worker.Stop(w)
	c.Check(err, jc.ErrorIsNil)
}

func (*WorkerSuite) TestStopReturnsError(c *gc.C) {
	w := workertest.NewDeadWorker(errors.New("pow"))

	err := worker.Stop(w)
	c.Check(err, gc.ErrorMatches, "pow")
}

func (*WorkerSuite) TestStopKills(c *gc.C) {
	w := workertest.NewErrorWorker(nil)
	defer workertest.CleanKill(c, w)

	worker.Stop(w)
	workertest.CheckKilled(c, w)
}

func (*WorkerSuite) TestStopWaits(c *gc.C) {
	w := workertest.NewForeverWorker(nil)
	defer workertest.CheckKilled(c, w)
	defer w.ReallyKill()

	done := make(chan struct{})
	go func() {
		defer close(done)
		worker.Stop(w)
	}()

	select {
	case <-time.After(coretesting.ShortWait):
	case <-done:
		c.Fatalf("Stop returned early")
	}

	w.ReallyKill()

	select {
	case <-done:
	case <-time.After(coretesting.LongWait):
		c.Fatalf("Stop never returned")
	}
}

func (*WorkerSuite) TestDeadAlready(c *gc.C) {
	w := workertest.NewDeadWorker(nil)

	select {
	case _, ok := <-worker.Dead(w):
		c.Check(ok, jc.IsFalse)
	case <-time.After(coretesting.LongWait):
		c.Fatalf("Dead never sent")
	}
}

func (*WorkerSuite) TestDeadWaits(c *gc.C) {
	w := workertest.NewErrorWorker(nil)
	defer workertest.CleanKill(c, w)

	dead := worker.Dead(w)
	select {
	case <-time.After(coretesting.ShortWait):
	case _, ok := <-dead:
		if !ok {
			c.Fatalf("Dead closed early")
		} else {
			c.Fatalf("Dead sent unexpectedly")
		}
	}

	w.Kill()
	select {
	case _, ok := <-dead:
		c.Check(ok, jc.IsFalse)
	case <-time.After(coretesting.LongWait):
		c.Fatalf("Dead never closed")
	}
}
