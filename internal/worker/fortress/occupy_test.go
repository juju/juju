// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package fortress_test

import (
	"context"
	"testing"
	"time"

	"github.com/juju/errors"
	"github.com/juju/tc"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/goleak"

	"github.com/juju/juju/internal/testhelpers"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/worker/fortress"
)

type OccupySuite struct {
	testhelpers.IsolationSuite
}

func TestOccupySuite(t *testing.T) {
	defer goleak.VerifyNone(t)
	tc.Run(t, &OccupySuite{})
}

func (*OccupySuite) TestAbort(c *tc.C) {
	fix := newFixture(c)
	defer fix.TearDown(c)

	// Try to occupy an unlocked fortress.
	run := func() (worker.Worker, error) {
		panic("shouldn't happen")
	}
	ctx, cancel := context.WithCancel(c.Context())
	defer cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		worker, err := fortress.Occupy(ctx, fix.Guest(c), run)
		c.Check(worker, tc.IsNil)
		c.Check(errors.Cause(err), tc.Equals, fortress.ErrAborted)
	}()

	// Observe that nothing happens.
	select {
	case <-done:
		c.Fatalf("started early")
	case <-time.After(coretesting.ShortWait):
	}

	// Abort and wait for completion.
	cancel()
	select {
	case <-done:
	case <-time.After(coretesting.LongWait):
		c.Fatalf("never cancelled")
	}
}

func (*OccupySuite) TestStartError(c *tc.C) {
	fix := newFixture(c)
	defer fix.TearDown(c)
	c.Check(fix.Guard(c).Unlock(c.Context()), tc.ErrorIsNil)

	// Error just passes straight through.
	run := func() (worker.Worker, error) {
		return nil, errors.New("splosh")
	}
	worker, err := fortress.Occupy(c.Context(), fix.Guest(c), run)
	c.Check(worker, tc.IsNil)
	c.Check(err, tc.ErrorMatches, "splosh")

	// Guard can lock fortress immediately.
	err = fix.Guard(c).Lockdown(c.Context())
	c.Check(err, tc.ErrorIsNil)
	AssertLocked(c, fix.Guest(c))
}

func (*OccupySuite) TestStartSuccess(c *tc.C) {
	fix := newFixture(c)
	defer fix.TearDown(c)
	c.Check(fix.Guard(c).Unlock(c.Context()), tc.ErrorIsNil)

	// Start a worker...
	expect := workertest.NewErrorWorker(nil)
	defer workertest.CleanKill(c, expect)
	run := func() (worker.Worker, error) {
		return expect, nil
	}
	worker, err := fortress.Occupy(c.Context(), fix.Guest(c), run)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(worker, tc.Equals, expect)

	// ...and check we can't lockdown again...
	locked := make(chan error, 1)
	go func() {
		locked <- fix.Guard(c).Lockdown(c.Context())
	}()
	select {
	case err := <-locked:
		c.Fatalf("unexpected Lockdown result: %v", err)
	case <-time.After(coretesting.ShortWait):
	}

	// ...until the worker completes.
	workertest.CleanKill(c, worker)
	select {
	case err := <-locked:
		c.Check(err, tc.ErrorIsNil)
	case <-time.After(coretesting.LongWait):
		c.Fatalf("visit never completed")
	}
}

func (*OccupySuite) TestSlowStartCancelContext(c *tc.C) {
	fix := newFixture(c)
	defer fix.TearDown(c)
	c.Check(fix.Guard(c).Unlock(c.Context()), tc.ErrorIsNil)

	// Start a worker...
	expect := workertest.NewErrorWorker(nil)
	defer workertest.CleanKill(c, expect)

	ctx, cancel := context.WithCancel(c.Context())
	ready := make(chan struct{})
	run := func() (worker.Worker, error) {
		cancel()
		<-ready
		return expect, nil
	}

	worker, err := fortress.Occupy(ctx, fix.Guest(c), run)
	c.Assert(err, tc.ErrorIs, fortress.ErrAborted)
	c.Check(worker, tc.IsNil)

	// ...and check we can't lockdown again...
	locked := make(chan error, 1)
	go func() {
		locked <- fix.Guard(c).Lockdown(c.Context())
	}()
	select {
	case err := <-locked:
		c.Fatalf("unexpected Lockdown result: %v", err)
	case <-time.After(coretesting.ShortWait):
	}

	// ...until the worker is killed because it did not
	// start before the Occupy context cancelled.
	close(ready)

	select {
	case err := <-locked:
		c.Check(err, tc.ErrorIsNil)
	case <-c.Context().Done():
		c.Fatalf("visit never completed")
	}
}
