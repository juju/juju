// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package fortress_test

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/fortress"
	"github.com/juju/juju/worker/workertest"
)

type OccupySuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&OccupySuite{})

func (*OccupySuite) TestAbort(c *gc.C) {
	fix := newFixture(c)
	defer fix.TearDown(c)

	// Try to occupy an unlocked fortress.
	run := func() (worker.Worker, error) {
		panic("shouldn't happen")
	}
	abort := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		worker, err := fortress.Occupy(fix.Guest(c), run, abort)
		c.Check(worker, gc.IsNil)
		c.Check(errors.Cause(err), gc.Equals, fortress.ErrAborted)
	}()

	// Observe that nothing happens.
	select {
	case <-done:
		c.Fatalf("started early")
	case <-time.After(coretesting.ShortWait):
	}

	// Abort and wait for completion.
	close(abort)
	select {
	case <-done:
	case <-time.After(coretesting.LongWait):
		c.Fatalf("never cancelled")
	}
}

func (*OccupySuite) TestStartError(c *gc.C) {
	fix := newFixture(c)
	defer fix.TearDown(c)
	c.Check(fix.Guard(c).Unlock(), jc.ErrorIsNil)

	// Error just passes straight through.
	run := func() (worker.Worker, error) {
		return nil, errors.New("splosh")
	}
	worker, err := fortress.Occupy(fix.Guest(c), run, nil)
	c.Check(worker, gc.IsNil)
	c.Check(err, gc.ErrorMatches, "splosh")

	// Guard can lock fortress immediately.
	err = fix.Guard(c).Lockdown(nil)
	c.Check(err, jc.ErrorIsNil)
	AssertLocked(c, fix.Guest(c))
}

func (*OccupySuite) TestStartSuccess(c *gc.C) {
	fix := newFixture(c)
	defer fix.TearDown(c)
	c.Check(fix.Guard(c).Unlock(), jc.ErrorIsNil)

	// Start a worker...
	expect := workertest.NewErrorWorker(nil)
	defer workertest.CleanKill(c, expect)
	run := func() (worker.Worker, error) {
		return expect, nil
	}
	worker, err := fortress.Occupy(fix.Guest(c), run, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(worker, gc.Equals, expect)

	// ...and check we can't lockdown again...
	locked := make(chan error, 1)
	go func() {
		locked <- fix.Guard(c).Lockdown(nil)
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
		c.Check(err, jc.ErrorIsNil)
	case <-time.After(coretesting.LongWait):
		c.Fatalf("visit never completed")
	}
}
