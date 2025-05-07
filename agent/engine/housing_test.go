// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package engine_test

import (
	"context"
	"time"

	"github.com/juju/errors"
	"github.com/juju/tc"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"
	dt "github.com/juju/worker/v4/dependency/testing"
	"github.com/juju/worker/v4/workertest"

	"github.com/juju/juju/agent/engine"
	"github.com/juju/juju/internal/testhelpers"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/worker/fortress"
)

type HousingSuite struct {
	testhelpers.IsolationSuite
}

var _ = tc.Suite(&HousingSuite{})

func (*HousingSuite) TestEmptyHousingEmptyManifold(c *tc.C) {
	manifold := engine.Housing{}.Decorate(dependency.Manifold{})

	c.Check(manifold.Inputs, tc.HasLen, 0)
	c.Check(manifold.Start, tc.IsNil)
	c.Check(manifold.Output, tc.IsNil)
	c.Check(manifold.Filter, tc.IsNil)
}

func (*HousingSuite) TestEmptyHousingPopulatedManifold(c *tc.C) {
	manifold := engine.Housing{}.Decorate(dependency.Manifold{
		Inputs: []string{"x", "y", "z"},
		Start:  panicStart,
		Output: panicOutput,
		Filter: panicFilter,
	})

	c.Check(manifold.Inputs, tc.DeepEquals, []string{"x", "y", "z"})
	c.Check(func() {
		manifold.Start(context.Background(), nil)
	}, tc.PanicMatches, "panicStart")
	c.Check(func() {
		manifold.Output(nil, nil)
	}, tc.PanicMatches, "panicOutput")
	c.Check(func() {
		manifold.Filter(nil)
	}, tc.PanicMatches, "panicFilter")
}

func (*HousingSuite) TestReplacesFilter(c *tc.C) {
	expectIn := errors.New("tweedledum")
	expectOut := errors.New("tweedledee")
	manifold := engine.Housing{
		Filter: func(in error) error {
			c.Check(in, tc.Equals, expectIn)
			return expectOut
		},
	}.Decorate(dependency.Manifold{
		Filter: panicFilter,
	})

	out := manifold.Filter(expectIn)
	c.Check(out, tc.Equals, expectOut)
}

func (*HousingSuite) TestFlagsNoInput(c *tc.C) {
	manifold := engine.Housing{
		Flags: []string{"foo", "bar"},
	}.Decorate(dependency.Manifold{})

	expect := []string{"foo", "bar"}
	c.Check(manifold.Inputs, tc.DeepEquals, expect)
}

func (*HousingSuite) TestFlagsNewInput(c *tc.C) {
	manifold := engine.Housing{
		Flags: []string{"foo", "bar"},
	}.Decorate(dependency.Manifold{
		Inputs: []string{"ping", "pong"},
	})

	expect := []string{"ping", "pong", "foo", "bar"}
	c.Check(manifold.Inputs, tc.DeepEquals, expect)
}

func (*HousingSuite) TestFlagsExistingInput(c *tc.C) {
	manifold := engine.Housing{
		Flags: []string{"a", "c", "d"},
	}.Decorate(dependency.Manifold{
		Inputs: []string{"a", "b"},
	})

	expect := []string{"a", "b", "c", "d"}
	c.Check(manifold.Inputs, tc.DeepEquals, expect)
}

func (*HousingSuite) TestFlagMissing(c *tc.C) {
	manifold := engine.Housing{
		Flags: []string{"flag"},
	}.Decorate(dependency.Manifold{})
	getter := dt.StubGetter(map[string]interface{}{
		"flag": dependency.ErrMissing,
	})

	worker, err := manifold.Start(context.Background(), getter)
	c.Check(worker, tc.IsNil)
	c.Check(errors.Cause(err), tc.Equals, dependency.ErrMissing)
}

func (*HousingSuite) TestFlagBadType(c *tc.C) {
	manifold := engine.Housing{
		Flags: []string{"flag"},
	}.Decorate(dependency.Manifold{})
	getter := dt.StubGetter(map[string]interface{}{
		"flag": false,
	})

	worker, err := manifold.Start(context.Background(), getter)
	c.Check(worker, tc.IsNil)
	c.Check(err, tc.ErrorMatches, "cannot set false into .*")
}

func (*HousingSuite) TestFlagBadValue(c *tc.C) {
	manifold := engine.Housing{
		Flags: []string{"flag"},
	}.Decorate(dependency.Manifold{})
	getter := dt.StubGetter(map[string]interface{}{
		"flag": flag{false},
	})

	worker, err := manifold.Start(context.Background(), getter)
	c.Check(worker, tc.IsNil)
	c.Check(errors.Cause(err), tc.Equals, dependency.ErrMissing)
}

func (*HousingSuite) TestFlagSuccess(c *tc.C) {
	expectWorker := &struct{ worker.Worker }{}
	manifold := engine.Housing{
		Flags: []string{"flag"},
	}.Decorate(dependency.Manifold{
		Start: func(ctx context.Context, getter dependency.Getter) (worker.Worker, error) {
			return expectWorker, nil
		},
	})
	getter := dt.StubGetter(map[string]interface{}{
		"flag": flag{true},
	})

	worker, err := manifold.Start(context.Background(), getter)
	c.Check(worker, tc.Equals, expectWorker)
	c.Check(err, tc.ErrorIsNil)
}

func (*HousingSuite) TestOccupyNewInput(c *tc.C) {
	manifold := engine.Housing{
		Occupy: "fortress",
	}.Decorate(dependency.Manifold{
		Inputs: []string{"ping", "pong"},
	})

	expect := []string{"ping", "pong", "fortress"}
	c.Check(manifold.Inputs, tc.DeepEquals, expect)
}

func (*HousingSuite) TestOccupyExistingInput(c *tc.C) {
	manifold := engine.Housing{
		Occupy: "fortress",
	}.Decorate(dependency.Manifold{
		Inputs: []string{"citadel", "fortress", "bastion"},
	})

	expect := []string{"citadel", "fortress", "bastion"}
	c.Check(manifold.Inputs, tc.DeepEquals, expect)
}

func (*HousingSuite) TestFlagBlocksOccupy(c *tc.C) {
	manifold := engine.Housing{
		Flags:  []string{"flag"},
		Occupy: "fortress",
	}.Decorate(dependency.Manifold{})
	getter := dt.StubGetter(map[string]interface{}{
		"flag":     dependency.ErrMissing,
		"fortress": errors.New("never happen"),
	})

	worker, err := manifold.Start(context.Background(), getter)
	c.Check(worker, tc.IsNil)
	c.Check(errors.Cause(err), tc.Equals, dependency.ErrMissing)
}

func (*HousingSuite) TestOccupyMissing(c *tc.C) {
	manifold := engine.Housing{
		Occupy: "fortress",
	}.Decorate(dependency.Manifold{})
	getter := dt.StubGetter(map[string]interface{}{
		"fortress": dependency.ErrMissing,
	})

	worker, err := manifold.Start(context.Background(), getter)
	c.Check(worker, tc.IsNil)
	c.Check(errors.Cause(err), tc.Equals, dependency.ErrMissing)
}

func (*HousingSuite) TestOccupyBadType(c *tc.C) {
	manifold := engine.Housing{
		Occupy: "fortress",
	}.Decorate(dependency.Manifold{})
	getter := dt.StubGetter(map[string]interface{}{
		"fortress": false,
	})

	worker, err := manifold.Start(context.Background(), getter)
	c.Check(worker, tc.IsNil)
	c.Check(err, tc.ErrorMatches, "cannot set false into .*")
}

func (*HousingSuite) TestOccupyLocked(c *tc.C) {
	manifold := engine.Housing{
		Occupy: "fortress",
	}.Decorate(dependency.Manifold{})

	ctx, cancel := context.WithCancel(context.Background())
	getter := dt.StubGetter(map[string]interface{}{
		"fortress": newGuest(false),
	})

	// start the start func
	started := make(chan struct{})
	go func() {
		defer close(started)
		worker, err := manifold.Start(ctx, getter)
		c.Check(worker, tc.IsNil)
		c.Check(errors.Cause(err), tc.Equals, fortress.ErrAborted)
	}()

	// check it's blocked...
	select {
	case <-time.After(coretesting.ShortWait):
	case <-started:
		c.Errorf("Start finished early")
	}

	// ...until the context is aborted.
	cancel()
	select {
	case <-started:
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out")
	}
}

func (*HousingSuite) TestOccupySuccess(c *tc.C) {
	expectWorker := workertest.NewErrorWorker(errors.New("ignored"))
	defer workertest.DirtyKill(c, expectWorker)
	manifold := engine.Housing{
		Occupy: "fortress",
	}.Decorate(dependency.Manifold{
		Start: func(ctx context.Context, getter dependency.Getter) (worker.Worker, error) {
			return expectWorker, nil
		},
	})
	guest := newGuest(true)
	getter := dt.StubGetter(map[string]interface{}{
		"fortress": guest,
	})

	// wait for the start func to complete
	started := make(chan struct{})
	go func() {
		defer close(started)
		worker, err := manifold.Start(context.Background(), getter)
		c.Check(worker, tc.Equals, expectWorker)
		c.Check(err, tc.ErrorIsNil)
	}()
	select {
	case <-started:
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out")
	}

	// check the worker's alive
	workertest.CheckAlive(c, expectWorker)

	// check the visit keeps running...
	select {
	case <-time.After(coretesting.ShortWait):
	case <-guest.done:
		c.Fatalf("visit finished early")
	}

	// ...until the worker stops
	expectWorker.Kill()
	select {
	case <-guest.done:
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out")
	}
}

func newGuest(unlocked bool) guest {
	return guest{
		unlocked: unlocked,
		done:     make(chan struct{}),
	}
}

// guest implements fortress.Guest.
type guest struct {
	unlocked bool
	done     chan struct{}
}

// Visit is part of the fortress.Guest interface.
func (guest guest) Visit(visit fortress.Visit, abort fortress.Abort) error {
	defer close(guest.done)
	if guest.unlocked {
		return visit()
	}
	<-abort
	return fortress.ErrAborted
}

// flag implements engine.Flag.
type flag struct {
	value bool
}

// Check is part of the engine.Flag interface.
func (flag flag) Check() bool {
	return flag.value
}

func panicStart(ctx context.Context, getter dependency.Getter) (worker.Worker, error) {
	panic("panicStart")
}

func panicOutput(worker.Worker, interface{}) error {
	panic("panicOutput")
}

func panicFilter(error) error {
	panic("panicFilter")
}
