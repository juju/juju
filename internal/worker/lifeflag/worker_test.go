// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lifeflag_test

import (
	"errors"
	"testing"

	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"github.com/juju/worker/v4/workertest"

	apilifeflag "github.com/juju/juju/api/controller/lifeflag"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/worker/lifeflag"
)

type WorkerSuite struct {
	testhelpers.IsolationSuite
}

func TestWorkerSuite(t *testing.T) {
	tc.Run(t, &WorkerSuite{})
}

func (*WorkerSuite) TestCreateNotFoundError(c *tc.C) {
	stub := &testhelpers.Stub{}
	stub.SetErrors(apilifeflag.ErrEntityNotFound)
	config := lifeflag.Config{
		Facade: newMockFacade(stub),
		Entity: testEntity,
		Result: explode,
	}

	worker, err := lifeflag.New(c.Context(), config)
	c.Check(worker, tc.IsNil)
	c.Check(err, tc.ErrorIs, apilifeflag.ErrEntityNotFound)
	checkCalls(c, stub, "Life")
}

func (*WorkerSuite) TestCreateRandomError(c *tc.C) {
	stub := &testhelpers.Stub{}
	stub.SetErrors(errors.New("boom splat"))
	config := lifeflag.Config{
		Facade: newMockFacade(stub),
		Entity: testEntity,
		Result: explode,
	}

	worker, err := lifeflag.New(c.Context(), config)
	c.Check(worker, tc.IsNil)
	c.Check(err, tc.ErrorMatches, "boom splat")
	checkCalls(c, stub, "Life")
}

func (*WorkerSuite) TestWatchNotFoundError(c *tc.C) {
	stub := &testhelpers.Stub{}
	stub.SetErrors(nil, apilifeflag.ErrEntityNotFound)
	config := lifeflag.Config{
		Facade: newMockFacade(stub, life.Alive),
		Entity: testEntity,
		Result: never,
	}

	worker, err := lifeflag.New(c.Context(), config)
	c.Check(err, tc.ErrorIsNil)
	c.Check(worker.Check(), tc.IsFalse)

	err = workertest.CheckKilled(c, worker)
	c.Check(err, tc.ErrorIs, apilifeflag.ErrEntityNotFound)
	checkCalls(c, stub, "Life", "Watch")
}

func (*WorkerSuite) TestWatchRandomError(c *tc.C) {
	stub := &testhelpers.Stub{}
	stub.SetErrors(nil, errors.New("pew pew"))
	config := lifeflag.Config{
		Facade: newMockFacade(stub, life.Alive),
		Entity: testEntity,
		Result: never,
	}

	worker, err := lifeflag.New(c.Context(), config)
	c.Check(err, tc.ErrorIsNil)
	c.Check(worker.Check(), tc.IsFalse)

	err = workertest.CheckKilled(c, worker)
	c.Check(err, tc.ErrorMatches, "pew pew")
	checkCalls(c, stub, "Life", "Watch")
}

func (*WorkerSuite) TestLifeNotFoundError(c *tc.C) {
	stub := &testhelpers.Stub{}
	stub.SetErrors(nil, nil, apilifeflag.ErrEntityNotFound)
	config := lifeflag.Config{
		Facade: newMockFacade(stub, life.Alive),
		Entity: testEntity,
		Result: never,
	}

	worker, err := lifeflag.New(c.Context(), config)
	c.Check(err, tc.ErrorIsNil)
	c.Check(worker.Check(), tc.IsFalse)

	err = workertest.CheckKilled(c, worker)
	c.Check(err, tc.ErrorIs, apilifeflag.ErrEntityNotFound)
	checkCalls(c, stub, "Life", "Watch", "Life")
}

func (*WorkerSuite) TestLifeRandomError(c *tc.C) {
	stub := &testhelpers.Stub{}
	stub.SetErrors(nil, nil, errors.New("rawr"))
	config := lifeflag.Config{
		Facade: newMockFacade(stub, life.Alive),
		Entity: testEntity,
		Result: never,
	}

	worker, err := lifeflag.New(c.Context(), config)
	c.Check(err, tc.ErrorIsNil)
	c.Check(worker.Check(), tc.IsFalse)

	err = workertest.CheckKilled(c, worker)
	c.Check(err, tc.ErrorMatches, "rawr")
	checkCalls(c, stub, "Life", "Watch", "Life")
}

func (*WorkerSuite) TestResultImmediateRealChange(c *tc.C) {
	stub := &testhelpers.Stub{}
	config := lifeflag.Config{
		Facade: newMockFacade(stub, life.Alive, life.Dead),
		Entity: testEntity,
		Result: life.IsNotAlive,
	}

	worker, err := lifeflag.New(c.Context(), config)
	c.Check(err, tc.ErrorIsNil)
	c.Check(worker.Check(), tc.IsFalse)

	err = workertest.CheckKilled(c, worker)
	c.Check(err, tc.Equals, lifeflag.ErrValueChanged)
	checkCalls(c, stub, "Life", "Watch", "Life")
}

func (*WorkerSuite) TestResultSubsequentRealChange(c *tc.C) {
	stub := &testhelpers.Stub{}
	config := lifeflag.Config{
		Facade: newMockFacade(stub, life.Dying, life.Dying, life.Dead),
		Entity: testEntity,
		Result: life.IsNotDead,
	}
	worker, err := lifeflag.New(c.Context(), config)
	c.Check(err, tc.ErrorIsNil)
	c.Check(worker.Check(), tc.IsTrue)

	err = workertest.CheckKilled(c, worker)
	c.Check(err, tc.Equals, lifeflag.ErrValueChanged)
	checkCalls(c, stub, "Life", "Watch", "Life", "Life")
}

func (*WorkerSuite) TestResultNoRealChange(c *tc.C) {
	stub := &testhelpers.Stub{}
	config := lifeflag.Config{
		Facade: newMockFacade(stub, life.Alive, life.Alive, life.Dying),
		Entity: testEntity,
		Result: life.IsNotDead,
	}
	worker, err := lifeflag.New(c.Context(), config)
	c.Check(err, tc.ErrorIsNil)
	c.Check(worker.Check(), tc.IsTrue)

	workertest.CheckAlive(c, worker)
	workertest.CleanKill(c, worker)
	checkCalls(c, stub, "Life", "Watch", "Life", "Life")
}

var testEntity = names.NewUnitTag("blah/123")

func checkCalls(c *tc.C, stub *testhelpers.Stub, names ...string) {
	stub.CheckCallNames(c, names...)
	for _, call := range stub.Calls() {
		c.Check(call.Args, tc.DeepEquals, []interface{}{testEntity})
	}
}

func explode(life.Value) bool { panic("unexpected") }
func never(life.Value) bool   { return false }
