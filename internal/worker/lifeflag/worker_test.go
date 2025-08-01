// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lifeflag_test

import (
	"errors"
	"time"

	"github.com/juju/names/v5"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v3/workertest"
	gc "gopkg.in/check.v1"

	apilifeflag "github.com/juju/juju/api/controller/lifeflag"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/internal/worker/lifeflag"
)

type WorkerSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&WorkerSuite{})

func (*WorkerSuite) TestCreateNotFoundError(c *gc.C) {
	stub := &testing.Stub{}
	stub.SetErrors(apilifeflag.ErrEntityNotFound)
	config := lifeflag.Config{
		Facade: newMockFacade(stub),
		Entity: testEntity,
		Result: explode,
	}

	worker, err := lifeflag.New(config)
	c.Check(worker, gc.IsNil)
	c.Check(errors.Is(err, apilifeflag.ErrEntityNotFound), jc.IsTrue)
	checkCalls(c, stub, "Life")
}

func (*WorkerSuite) TestCreateRandomError(c *gc.C) {
	stub := &testing.Stub{}
	stub.SetErrors(errors.New("boom splat"))
	config := lifeflag.Config{
		Facade: newMockFacade(stub),
		Entity: testEntity,
		Result: explode,
	}

	worker, err := lifeflag.New(config)
	c.Check(worker, gc.IsNil)
	c.Check(err, gc.ErrorMatches, "boom splat")
	checkCalls(c, stub, "Life")
}

func (*WorkerSuite) TestWatchNotFoundError(c *gc.C) {
	stub := &testing.Stub{}
	stub.SetErrors(nil, apilifeflag.ErrEntityNotFound)
	config := lifeflag.Config{
		Facade: newMockFacade(stub, alive),
		Entity: testEntity,
		Result: never,
	}

	worker, err := lifeflag.New(config)
	c.Check(err, jc.ErrorIsNil)
	c.Check(worker.Check(), jc.IsFalse)

	err = workertest.CheckKilled(c, worker)
	c.Check(errors.Is(err, apilifeflag.ErrEntityNotFound), jc.IsTrue)
	checkCalls(c, stub, "Life", "Watch")
}

func (*WorkerSuite) TestWatchRandomError(c *gc.C) {
	stub := &testing.Stub{}
	stub.SetErrors(nil, errors.New("pew pew"))
	config := lifeflag.Config{
		Facade: newMockFacade(stub, alive),
		Entity: testEntity,
		Result: never,
	}

	worker, err := lifeflag.New(config)
	c.Check(err, jc.ErrorIsNil)
	c.Check(worker.Check(), jc.IsFalse)

	err = workertest.CheckKilled(c, worker)
	c.Check(err, gc.ErrorMatches, "pew pew")
	checkCalls(c, stub, "Life", "Watch")
}

func (*WorkerSuite) TestLifeNotFoundError(c *gc.C) {
	stub := &testing.Stub{}
	stub.SetErrors(nil, nil, apilifeflag.ErrEntityNotFound)
	config := lifeflag.Config{
		Facade: newMockFacade(stub, alive),
		Entity: testEntity,
		Result: never,
	}

	worker, err := lifeflag.New(config)
	c.Check(err, jc.ErrorIsNil)
	c.Check(worker.Check(), jc.IsFalse)

	err = workertest.CheckKilled(c, worker)
	c.Check(errors.Is(err, apilifeflag.ErrEntityNotFound), jc.IsTrue)
	checkCalls(c, stub, "Life", "Watch", "Life")
}

func (*WorkerSuite) TestLifeRandomError(c *gc.C) {
	stub := &testing.Stub{}
	stub.SetErrors(nil, nil, errors.New("rawr"))
	config := lifeflag.Config{
		Facade: newMockFacade(stub, alive),
		Entity: testEntity,
		Result: never,
	}

	worker, err := lifeflag.New(config)
	c.Check(err, jc.ErrorIsNil)
	c.Check(worker.Check(), jc.IsFalse)

	err = workertest.CheckKilled(c, worker)
	c.Check(err, gc.ErrorMatches, "rawr")
	checkCalls(c, stub, "Life", "Watch", "Life")
}

func (*WorkerSuite) TestResultImmediateRealChange(c *gc.C) {
	done := make(chan struct{})

	stub := &testing.Stub{}
	config := lifeflag.Config{
		Facade: newMockFacade(stub, alive, func() life.Value {
			close(done)
			return life.Dead
		}),
		Entity: testEntity,
		Result: life.IsNotAlive,
	}

	worker, err := lifeflag.New(config)
	c.Check(err, jc.ErrorIsNil)
	c.Check(worker.Check(), jc.IsFalse)

	select {
	case <-done:
	case <-time.After(testing.LongWait):
		c.Fatal("timed out waiting for worker to change state")
	}

	// Now check that the life has actually changed!
	c.Check(worker.Check(), jc.IsTrue)

	err = workertest.CheckKilled(c, worker)
	c.Check(err, gc.Equals, lifeflag.ErrValueChanged)
	checkCalls(c, stub, "Life", "Watch", "Life")
}

func (*WorkerSuite) TestResultSubsequentRealChange(c *gc.C) {
	done := make(chan struct{})

	stub := &testing.Stub{}
	config := lifeflag.Config{
		Facade: newMockFacade(stub, dying, dying, func() life.Value {
			close(done)
			return life.Dead
		}),
		Entity: testEntity,
		Result: life.IsNotDead,
	}
	worker, err := lifeflag.New(config)
	c.Check(err, jc.ErrorIsNil)
	c.Check(worker.Check(), jc.IsTrue)

	select {
	case <-done:
	case <-time.After(testing.LongWait):
		c.Fatal("timed out waiting for worker to change state")
	}

	// Now check that the life has actually changed!
	c.Check(worker.Check(), jc.IsFalse)

	err = workertest.CheckKilled(c, worker)
	c.Check(err, gc.Equals, lifeflag.ErrValueChanged)
	checkCalls(c, stub, "Life", "Watch", "Life", "Life")
}

func (*WorkerSuite) TestResultNoRealChange(c *gc.C) {
	stub := &testing.Stub{}
	config := lifeflag.Config{
		Facade: newMockFacade(stub, alive, alive, dying),
		Entity: testEntity,
		Result: life.IsNotDead,
	}
	worker, err := lifeflag.New(config)
	c.Check(err, jc.ErrorIsNil)
	c.Check(worker.Check(), jc.IsTrue)

	workertest.CheckAlive(c, worker)
	workertest.CleanKill(c, worker)
	checkCalls(c, stub, "Life", "Watch", "Life", "Life")
}

var testEntity = names.NewUnitTag("blah/123")

func checkCalls(c *gc.C, stub *testing.Stub, names ...string) {
	stub.CheckCallNames(c, names...)
	for _, call := range stub.Calls() {
		c.Check(call.Args, gc.DeepEquals, []interface{}{testEntity})
	}
}

func explode(life.Value) bool { panic("unexpected") }

func never(life.Value) bool { return false }

func alive() life.Value {
	return life.Alive
}

func dying() life.Value {
	return life.Dying
}
