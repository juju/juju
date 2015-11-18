// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package catacomb_test

import (
	"time"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"launchpad.net/tomb"

	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/catacomb"
)

type cleaner interface {
	AddCleanup(testing.CleanupFunc)
}

type fixture struct {
	catacomb catacomb.Catacomb
	cleaner  cleaner
}

func (fix *fixture) run(c *gc.C, task func(), init ...worker.Worker) error {
	err := catacomb.Invoke(catacomb.Plan{
		Site: &fix.catacomb,
		Work: func() error { task(); return nil },
		Init: init,
	})
	c.Assert(err, jc.ErrorIsNil)

	select {
	case <-fix.catacomb.Dead():
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out")
	}
	return fix.catacomb.Wait()
}

func (fix *fixture) waitDying(c *gc.C) {
	select {
	case <-fix.catacomb.Dying():
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out; still alive")
	}
}

func (fix *fixture) assertDying(c *gc.C) {
	select {
	case <-fix.catacomb.Dying():
	default:
		c.Fatalf("still alive")
	}
}

func (fix *fixture) assertNotDying(c *gc.C) {
	select {
	case <-fix.catacomb.Dying():
		c.Fatalf("already dying")
	default:
	}
}

func (fix *fixture) assertDead(c *gc.C) {
	select {
	case <-fix.catacomb.Dead():
	default:
		c.Fatalf("not dead")
	}
}

func (fix *fixture) assertNotDead(c *gc.C) {
	select {
	case <-fix.catacomb.Dead():
		c.Fatalf("already dead")
	default:
	}
}

func (fix *fixture) assertAddAlive(c *gc.C, w *errorWorker) {
	err := fix.catacomb.Add(w)
	c.Assert(err, jc.ErrorIsNil)
	w.waitStillAlive(c)
}

func (fix *fixture) startErrorWorker(c *gc.C, err error) *errorWorker {
	ew := &errorWorker{}
	go func() {
		defer ew.tomb.Done()
		defer ew.tomb.Kill(err)
		<-ew.tomb.Dying()
	}()
	fix.cleaner.AddCleanup(func(_ *gc.C) {
		ew.stop()
	})
	return ew
}

type errorWorker struct {
	tomb tomb.Tomb
}

func (ew *errorWorker) Kill() {
	ew.tomb.Kill(nil)
}

func (ew *errorWorker) Wait() error {
	return ew.tomb.Wait()
}

func (ew *errorWorker) stop() {
	ew.Kill()
	ew.Wait()
}

func (ew *errorWorker) waitStillAlive(c *gc.C) {
	select {
	case <-ew.tomb.Dying():
		c.Fatalf("already dying")
	case <-time.After(coretesting.ShortWait):
	}
}

func (ew *errorWorker) assertDead(c *gc.C) {
	select {
	case <-ew.tomb.Dead():
	default:
		c.Fatalf("not yet dead")
	}
}
