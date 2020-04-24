// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationflag_test

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v2/workertest"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/migration"
	"github.com/juju/juju/worker/migrationflag"
)

type WorkerSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&WorkerSuite{})

func (*WorkerSuite) TestPhaseErrorOnStartup(c *gc.C) {
	stub := &testing.Stub{}
	stub.SetErrors(errors.New("gaah"))
	facade := newMockFacade(stub)
	config := migrationflag.Config{
		Facade: facade,
		Model:  validUUID,
		Check:  panicCheck,
	}
	worker, err := migrationflag.New(config)
	c.Check(worker, gc.IsNil)
	c.Check(err, gc.ErrorMatches, "gaah")
	checkCalls(c, stub, "Phase")
}

func (*WorkerSuite) TestWatchError(c *gc.C) {
	stub := &testing.Stub{}
	stub.SetErrors(nil, errors.New("boff"))
	facade := newMockFacade(stub, migration.REAP)
	config := migrationflag.Config{
		Facade: facade,
		Model:  validUUID,
		Check:  neverCheck,
	}
	worker, err := migrationflag.New(config)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(worker.Check(), jc.IsFalse)

	err = workertest.CheckKilled(c, worker)
	c.Check(err, gc.ErrorMatches, "boff")
	checkCalls(c, stub, "Phase", "Watch")
}

func (*WorkerSuite) TestPhaseErrorWhileRunning(c *gc.C) {
	stub := &testing.Stub{}
	stub.SetErrors(nil, nil, errors.New("glug"))
	facade := newMockFacade(stub, migration.QUIESCE)
	config := migrationflag.Config{
		Facade: facade,
		Model:  validUUID,
		Check:  neverCheck,
	}
	worker, err := migrationflag.New(config)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(worker.Check(), jc.IsFalse)

	err = workertest.CheckKilled(c, worker)
	c.Check(err, gc.ErrorMatches, "glug")
	checkCalls(c, stub, "Phase", "Watch", "Phase")
}

func (*WorkerSuite) TestImmediatePhaseChange(c *gc.C) {
	stub := &testing.Stub{}
	facade := newMockFacade(stub,
		migration.QUIESCE, migration.REAP,
	)
	config := migrationflag.Config{
		Facade: facade,
		Model:  validUUID,
		Check:  isQuiesce,
	}
	worker, err := migrationflag.New(config)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(worker.Check(), jc.IsTrue)

	err = workertest.CheckKilled(c, worker)
	c.Check(err, gc.Equals, migrationflag.ErrChanged)
	checkCalls(c, stub, "Phase", "Watch", "Phase")
}

func (*WorkerSuite) TestSubsequentPhaseChange(c *gc.C) {
	stub := &testing.Stub{}
	facade := newMockFacade(stub,
		migration.ABORT, migration.REAP, migration.QUIESCE,
	)
	config := migrationflag.Config{
		Facade: facade,
		Model:  validUUID,
		Check:  isQuiesce,
	}
	worker, err := migrationflag.New(config)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(worker.Check(), jc.IsFalse)

	err = workertest.CheckKilled(c, worker)
	c.Check(err, gc.Equals, migrationflag.ErrChanged)
	checkCalls(c, stub, "Phase", "Watch", "Phase", "Phase")
}

func (*WorkerSuite) TestNoRelevantPhaseChange(c *gc.C) {
	stub := &testing.Stub{}
	facade := newMockFacade(stub,
		migration.REAPFAILED,
		migration.DONE,
		migration.ABORT,
		migration.IMPORT,
	)
	config := migrationflag.Config{
		Facade: facade,
		Model:  validUUID,
		Check:  isQuiesce,
	}
	worker, err := migrationflag.New(config)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(worker.Check(), jc.IsFalse)

	workertest.CheckAlive(c, worker)
	workertest.CleanKill(c, worker)
	checkCalls(c, stub, "Phase", "Watch", "Phase", "Phase", "Phase")
}

func (*WorkerSuite) TestIsTerminal(c *gc.C) {
	tests := []struct {
		phase    migration.Phase
		expected bool
	}{
		{migration.QUIESCE, false},
		{migration.SUCCESS, false},
		{migration.ABORT, false},
		{migration.NONE, true},
		{migration.UNKNOWN, true},
		{migration.ABORTDONE, true},
		{migration.DONE, true},
	}
	for _, t := range tests {
		c.Check(migrationflag.IsTerminal(t.phase), gc.Equals, t.expected,
			gc.Commentf("for %s", t.phase))
	}
}
