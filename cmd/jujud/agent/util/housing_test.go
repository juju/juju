// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package util_test

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/jujud/agent/util"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/dependency"
	dt "github.com/juju/juju/worker/dependency/testing"
	"github.com/juju/juju/worker/fortress"
	"github.com/juju/juju/worker/workertest"
)

type HousingSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&HousingSuite{})

func (*HousingSuite) TestEmptyHousingEmptyManifold(c *gc.C) {
	manifold := util.Housing{}.Decorate(dependency.Manifold{})

	c.Check(manifold.Inputs, gc.HasLen, 0)
	c.Check(manifold.Start, gc.IsNil)
	c.Check(manifold.Output, gc.IsNil)
	c.Check(manifold.Filter, gc.IsNil)
}

func (*HousingSuite) TestEmptyHousingPopulatedManifold(c *gc.C) {
	manifold := util.Housing{}.Decorate(dependency.Manifold{
		Inputs: []string{"x", "y", "z"},
		Start:  panicStart,
		Output: panicOutput,
		Filter: panicFilter,
	})

	c.Check(manifold.Inputs, jc.DeepEquals, []string{"x", "y", "z"})
	c.Check(func() {
		manifold.Start(nil)
	}, gc.PanicMatches, "panicStart")
	c.Check(func() {
		manifold.Output(nil, nil)
	}, gc.PanicMatches, "panicOutput")
	c.Check(func() {
		manifold.Filter(nil)
	}, gc.PanicMatches, "panicFilter")
}

func (*HousingSuite) TestReplacesFilter(c *gc.C) {
	expectIn := errors.New("tweedledum")
	expectOut := errors.New("tweedledee")
	manifold := util.Housing{
		Filter: func(in error) error {
			c.Check(in, gc.Equals, expectIn)
			return expectOut
		},
	}.Decorate(dependency.Manifold{
		Filter: panicFilter,
	})

	out := manifold.Filter(expectIn)
	c.Check(out, gc.Equals, expectOut)
}

func (*HousingSuite) TestFlagsNoInput(c *gc.C) {
	manifold := util.Housing{
		Flags: []string{"foo", "bar"},
	}.Decorate(dependency.Manifold{})

	expect := []string{"foo", "bar"}
	c.Check(manifold.Inputs, jc.DeepEquals, expect)
}

func (*HousingSuite) TestFlagsNewInput(c *gc.C) {
	manifold := util.Housing{
		Flags: []string{"foo", "bar"},
	}.Decorate(dependency.Manifold{
		Inputs: []string{"ping", "pong"},
	})

	expect := []string{"ping", "pong", "foo", "bar"}
	c.Check(manifold.Inputs, jc.DeepEquals, expect)
}

func (*HousingSuite) TestFlagsExistingInput(c *gc.C) {
	manifold := util.Housing{
		Flags: []string{"a", "c", "d"},
	}.Decorate(dependency.Manifold{
		Inputs: []string{"a", "b"},
	})

	expect := []string{"a", "b", "c", "d"}
	c.Check(manifold.Inputs, jc.DeepEquals, expect)
}

func (*HousingSuite) TestFlagMissing(c *gc.C) {
	manifold := util.Housing{
		Flags: []string{"flag"},
	}.Decorate(dependency.Manifold{})
	context := dt.StubContext(nil, map[string]interface{}{
		"flag": dependency.ErrMissing,
	})

	worker, err := manifold.Start(context)
	c.Check(worker, gc.IsNil)
	c.Check(errors.Cause(err), gc.Equals, dependency.ErrMissing)
}

func (*HousingSuite) TestFlagBadType(c *gc.C) {
	manifold := util.Housing{
		Flags: []string{"flag"},
	}.Decorate(dependency.Manifold{})
	context := dt.StubContext(nil, map[string]interface{}{
		"flag": false,
	})

	worker, err := manifold.Start(context)
	c.Check(worker, gc.IsNil)
	c.Check(err, gc.ErrorMatches, "cannot set false into .*")
}

func (*HousingSuite) TestFlagBadValue(c *gc.C) {
	manifold := util.Housing{
		Flags: []string{"flag"},
	}.Decorate(dependency.Manifold{})
	context := dt.StubContext(nil, map[string]interface{}{
		"flag": flag{false},
	})

	worker, err := manifold.Start(context)
	c.Check(worker, gc.IsNil)
	c.Check(errors.Cause(err), gc.Equals, dependency.ErrMissing)
}

func (*HousingSuite) TestFlagSuccess(c *gc.C) {
	expectWorker := &struct{ worker.Worker }{}
	manifold := util.Housing{
		Flags: []string{"flag"},
	}.Decorate(dependency.Manifold{
		Start: func(dependency.Context) (worker.Worker, error) {
			return expectWorker, nil
		},
	})
	context := dt.StubContext(nil, map[string]interface{}{
		"flag": flag{true},
	})

	worker, err := manifold.Start(context)
	c.Check(worker, gc.Equals, expectWorker)
	c.Check(err, jc.ErrorIsNil)
}

func (*HousingSuite) TestOccupyNewInput(c *gc.C) {
	manifold := util.Housing{
		Occupy: "fortress",
	}.Decorate(dependency.Manifold{
		Inputs: []string{"ping", "pong"},
	})

	expect := []string{"ping", "pong", "fortress"}
	c.Check(manifold.Inputs, jc.DeepEquals, expect)
}

func (*HousingSuite) TestOccupyExistingInput(c *gc.C) {
	manifold := util.Housing{
		Occupy: "fortress",
	}.Decorate(dependency.Manifold{
		Inputs: []string{"citadel", "fortress", "bastion"},
	})

	expect := []string{"citadel", "fortress", "bastion"}
	c.Check(manifold.Inputs, jc.DeepEquals, expect)
}

func (*HousingSuite) TestFlagBlocksOccupy(c *gc.C) {
	manifold := util.Housing{
		Flags:  []string{"flag"},
		Occupy: "fortress",
	}.Decorate(dependency.Manifold{})
	context := dt.StubContext(nil, map[string]interface{}{
		"flag":     dependency.ErrMissing,
		"fortress": errors.New("never happen"),
	})

	worker, err := manifold.Start(context)
	c.Check(worker, gc.IsNil)
	c.Check(errors.Cause(err), gc.Equals, dependency.ErrMissing)
}

func (*HousingSuite) TestOccupyMissing(c *gc.C) {
	manifold := util.Housing{
		Occupy: "fortress",
	}.Decorate(dependency.Manifold{})
	context := dt.StubContext(nil, map[string]interface{}{
		"fortress": dependency.ErrMissing,
	})

	worker, err := manifold.Start(context)
	c.Check(worker, gc.IsNil)
	c.Check(errors.Cause(err), gc.Equals, dependency.ErrMissing)
}

func (*HousingSuite) TestOccupyBadType(c *gc.C) {
	manifold := util.Housing{
		Occupy: "fortress",
	}.Decorate(dependency.Manifold{})
	context := dt.StubContext(nil, map[string]interface{}{
		"fortress": false,
	})

	worker, err := manifold.Start(context)
	c.Check(worker, gc.IsNil)
	c.Check(err, gc.ErrorMatches, "cannot set false into .*")
}

func (*HousingSuite) TestOccupyLocked(c *gc.C) {
	manifold := util.Housing{
		Occupy: "fortress",
	}.Decorate(dependency.Manifold{})
	abort := make(chan struct{})
	context := dt.StubContext(abort, map[string]interface{}{
		"fortress": newGuest(false),
	})

	// start the start func
	started := make(chan struct{})
	go func() {
		defer close(started)
		worker, err := manifold.Start(context)
		c.Check(worker, gc.IsNil)
		c.Check(errors.Cause(err), gc.Equals, fortress.ErrAborted)
	}()

	// check it's blocked...
	select {
	case <-time.After(coretesting.ShortWait):
	case <-started:
		c.Errorf("Start finished early")
	}

	// ...until the context is aborted.
	close(abort)
	select {
	case <-started:
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out")
	}
}

func (*HousingSuite) TestOccupySuccess(c *gc.C) {
	expectWorker := workertest.NewErrorWorker(errors.New("ignored"))
	defer workertest.DirtyKill(c, expectWorker)
	manifold := util.Housing{
		Occupy: "fortress",
	}.Decorate(dependency.Manifold{
		Start: func(dependency.Context) (worker.Worker, error) {
			return expectWorker, nil
		},
	})
	guest := newGuest(true)
	context := dt.StubContext(nil, map[string]interface{}{
		"fortress": guest,
	})

	// wait for the start func to complete
	started := make(chan struct{})
	go func() {
		defer close(started)
		worker, err := manifold.Start(context)
		c.Check(worker, gc.Equals, expectWorker)
		c.Check(err, jc.ErrorIsNil)
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

// flag implements util.Flag.
type flag struct {
	value bool
}

// Check is part of the util.Flag interface.
func (flag flag) Check() bool {
	return flag.value
}

func panicStart(dependency.Context) (worker.Worker, error) {
	panic("panicStart")
}

func panicOutput(worker.Worker, interface{}) error {
	panic("panicOutput")
}

func panicFilter(error) error {
	panic("panicFilter")
}
