// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package fortress_test

import (
	"context"
	"time"

	"github.com/juju/errors"
	"github.com/juju/tc"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"

	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/worker/fortress"
)

// fixture holds a fortress worker and the manifold whence it sprang.
type fixture struct {
	manifold dependency.Manifold
	worker   worker.Worker
}

// newFixture returns a new fixture with a running worker. The caller
// takes responsibility for stopping the worker (most easily accomplished
// by deferring a TearDown).
func newFixture(c *tc.C) *fixture {
	manifold := fortress.Manifold()
	worker, err := manifold.Start(c.Context(), nil)
	c.Assert(err, tc.ErrorIsNil)
	return &fixture{
		manifold: manifold,
		worker:   worker,
	}
}

// TearDown stops the worker and checks it encountered no errors.
func (fix *fixture) TearDown(c *tc.C) {
	CheckStop(c, fix.worker)
}

// Guard returns a fortress.Guard backed by the fixture's worker.
func (fix *fixture) Guard(c *tc.C) (out fortress.Guard) {
	err := fix.manifold.Output(fix.worker, &out)
	c.Assert(err, tc.ErrorIsNil)
	return out
}

// Guest returns a fortress.Guest backed by the fixture's worker.
func (fix *fixture) Guest(c *tc.C) (out fortress.Guest) {
	err := fix.manifold.Output(fix.worker, &out)
	c.Assert(err, tc.ErrorIsNil)
	return out
}

// startBlockingVisit Unlocks the fortress; starts a Visit and waits for it to
// be invoked; then leaves that Visit blocking, and returns a channel on which
// you (1) *can* send a value to unblock the visit but (2) *must* defer a close
// (in case your test fails before sending, in which case we still want to stop
// the visit).
func (fix *fixture) startBlockingVisit(c *tc.C) chan<- struct{} {
	err := fix.Guard(c).Unlock(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	visitStarted := make(chan struct{}, 1)
	defer close(visitStarted)

	unblockVisit := make(chan struct{}, 1)
	go func() {
		err := fix.Guest(c).Visit(c.Context(), func() error {
			select {
			case visitStarted <- struct{}{}:
			case <-time.After(coretesting.LongWait):
				c.Fatalf("visit never started sending")
			}

			// Block until the test closes the channel.
			select {
			case <-unblockVisit:
			case <-time.After(coretesting.LongWait):
				c.Fatalf("visit never unblocked - did you forget to close the channel?")
			}
			return nil
		})
		c.Check(err, tc.ErrorIsNil)
	}()
	select {
	case <-visitStarted:
	case <-time.After(coretesting.LongWait):
		c.Fatalf("visit never started reading")
	}

	return unblockVisit
}

// AssertUnlocked checks that the supplied Guest can Visit its fortress.
func AssertUnlocked(c *tc.C, guest fortress.Guest) {
	visited := make(chan error)
	go func() {
		visited <- guest.Visit(c.Context(), badVisit)
	}()

	select {
	case err := <-visited:
		c.Assert(err, tc.ErrorMatches, "bad!")
	case <-time.After(coretesting.LongWait):
		c.Fatalf("abort never handled")
	}
}

// AssertUnlocked checks that the supplied Guest's Visit calls are blocked
// (and can be cancelled via Abort).
func AssertLocked(c *tc.C, guest fortress.Guest) {
	visited := make(chan error)

	ctx, cancel := context.WithCancel(c.Context())
	defer cancel()

	go func() {
		visited <- guest.Visit(ctx, badVisit)
	}()

	// NOTE(fwereade): this isn't about interacting with a timer; it's about
	// making sure other goroutines have had ample opportunity to do stuff.
	delay := time.After(coretesting.ShortWait)
	for {
		select {
		case <-delay:
			delay = nil
			cancel()
		case err := <-visited:
			c.Assert(err, tc.Equals, fortress.ErrAborted)
			return
		case <-time.After(coretesting.LongWait):
			c.Fatalf("timed out")
		}
	}
}

// CheckStop stops the worker and checks it encountered no error.
func CheckStop(c *tc.C, w worker.Worker) {
	c.Check(worker.Stop(w), tc.ErrorIsNil)
}

// badVisit is a Vist that always fails.
func badVisit() error {
	return errors.New("bad!")
}
