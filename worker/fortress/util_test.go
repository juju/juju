// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package fortress_test

import (
	"time"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v2"
	"github.com/juju/worker/v2/dependency"
	gc "gopkg.in/check.v1"

	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/fortress"
)

// fixture holds a fortress worker and the manifold whence it sprang.
type fixture struct {
	manifold dependency.Manifold
	worker   worker.Worker
}

// newFixture returns a new fixture with a running worker. The caller
// takes responsibility for stopping the worker (most easily accomplished
// by deferring a TearDown).
func newFixture(c *gc.C) *fixture {
	manifold := fortress.Manifold()
	worker, err := manifold.Start(nil)
	c.Assert(err, jc.ErrorIsNil)
	return &fixture{
		manifold: manifold,
		worker:   worker,
	}
}

// TearDown stops the worker and checks it encountered no errors.
func (fix *fixture) TearDown(c *gc.C) {
	CheckStop(c, fix.worker)
}

// Guard returns a fortress.Guard backed by the fixture's worker.
func (fix *fixture) Guard(c *gc.C) (out fortress.Guard) {
	err := fix.manifold.Output(fix.worker, &out)
	c.Assert(err, jc.ErrorIsNil)
	return out
}

// Guest returns a fortress.Guest backed by the fixture's worker.
func (fix *fixture) Guest(c *gc.C) (out fortress.Guest) {
	err := fix.manifold.Output(fix.worker, &out)
	c.Assert(err, jc.ErrorIsNil)
	return out
}

// startBlockingVisit Unlocks the fortress; starts a Visit and waits for it to
// be invoked; then leaves that Visit blocking, and returns a channel on which
// you (1) *can* send a value to unblock the visit but (2) *must* defer a close
// (in case your test fails before sending, in which case we still want to stop
// the visit).
func (fix *fixture) startBlockingVisit(c *gc.C) chan<- struct{} {
	err := fix.Guard(c).Unlock()
	c.Assert(err, jc.ErrorIsNil)
	visitStarted := make(chan struct{}, 1)
	defer close(visitStarted)
	unblockVisit := make(chan struct{}, 1)
	go func() {
		err := fix.Guest(c).Visit(func() error {
			visitStarted <- struct{}{}
			<-unblockVisit
			return nil
		}, nil)
		c.Check(err, jc.ErrorIsNil)
	}()
	select {
	case <-visitStarted:
	case <-time.After(coretesting.LongWait):
		c.Fatalf("visit never started")
	}
	return unblockVisit
}

// AssertUnlocked checks that the supplied Guest can Visit its fortress.
func AssertUnlocked(c *gc.C, guest fortress.Guest) {
	visited := make(chan error)
	go func() {
		visited <- guest.Visit(badVisit, nil)
	}()

	select {
	case err := <-visited:
		c.Assert(err, gc.ErrorMatches, "bad!")
	case <-time.After(coretesting.LongWait):
		c.Fatalf("abort never handled")
	}
}

// AssertUnlocked checks that the supplied Guest's Visit calls are blocked
// (and can be cancelled via Abort).
func AssertLocked(c *gc.C, guest fortress.Guest) {
	visited := make(chan error)
	abort := make(chan struct{})
	go func() {
		visited <- guest.Visit(badVisit, abort)
	}()

	// NOTE(fwereade): this isn't about interacting with a timer; it's about
	// making sure other goroutines have had ample opportunity to do stuff.
	delay := time.After(coretesting.ShortWait)
	for {
		select {
		case <-delay:
			delay = nil
			close(abort)
		case err := <-visited:
			c.Assert(err, gc.Equals, fortress.ErrAborted)
			return
		case <-time.After(coretesting.LongWait):
			c.Fatalf("timed out")
		}
	}
}

// CheckStop stops the worker and checks it encountered no error.
func CheckStop(c *gc.C, w worker.Worker) {
	c.Check(worker.Stop(w), jc.ErrorIsNil)
}

// badVisit is a Vist that always fails.
func badVisit() error {
	return errors.New("bad!")
}
