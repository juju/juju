// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationflag_test

import (
	"github.com/juju/errors"
	"github.com/juju/tc"
	"github.com/juju/worker/v4/workertest"

	"github.com/juju/juju/core/migration"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/worker/migrationflag"
)

type WorkerSuite struct {
	testhelpers.IsolationSuite
}

var _ = tc.Suite(&WorkerSuite{})

func (*WorkerSuite) TestPhaseErrorOnStartup(c *tc.C) {
	stub := &testhelpers.Stub{}
	stub.SetErrors(errors.New("gaah"))
	facade := newMockFacade(stub)
	config := migrationflag.Config{
		Facade: facade,
		Model:  validUUID,
		Check:  panicCheck,
	}
	worker, err := migrationflag.New(c.Context(), config)
	c.Check(worker, tc.IsNil)
	c.Check(err, tc.ErrorMatches, "gaah")
	checkCalls(c, stub, "Phase")
}

func (*WorkerSuite) TestWatchError(c *tc.C) {
	stub := &testhelpers.Stub{}
	stub.SetErrors(nil, errors.New("boff"))
	facade := newMockFacade(stub, migration.REAP)
	config := migrationflag.Config{
		Facade: facade,
		Model:  validUUID,
		Check:  neverCheck,
	}
	worker, err := migrationflag.New(c.Context(), config)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(worker.Check(), tc.IsFalse)

	err = workertest.CheckKilled(c, worker)
	c.Check(err, tc.ErrorMatches, "boff")
	checkCalls(c, stub, "Phase", "Watch")
}

func (*WorkerSuite) TestPhaseErrorWhileRunning(c *tc.C) {
	stub := &testhelpers.Stub{}
	stub.SetErrors(nil, nil, errors.New("glug"))
	facade := newMockFacade(stub, migration.QUIESCE)
	config := migrationflag.Config{
		Facade: facade,
		Model:  validUUID,
		Check:  neverCheck,
	}
	worker, err := migrationflag.New(c.Context(), config)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(worker.Check(), tc.IsFalse)

	err = workertest.CheckKilled(c, worker)
	c.Check(err, tc.ErrorMatches, "glug")
	checkCalls(c, stub, "Phase", "Watch", "Phase")
}

func (*WorkerSuite) TestImmediatePhaseChange(c *tc.C) {
	stub := &testhelpers.Stub{}
	facade := newMockFacade(stub,
		migration.QUIESCE, migration.REAP,
	)
	config := migrationflag.Config{
		Facade: facade,
		Model:  validUUID,
		Check:  isQuiesce,
	}
	worker, err := migrationflag.New(c.Context(), config)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(worker.Check(), tc.IsTrue)

	err = workertest.CheckKilled(c, worker)
	c.Check(err, tc.Equals, migrationflag.ErrChanged)
	checkCalls(c, stub, "Phase", "Watch", "Phase")
}

func (*WorkerSuite) TestSubsequentPhaseChange(c *tc.C) {
	stub := &testhelpers.Stub{}
	facade := newMockFacade(stub,
		migration.ABORT, migration.REAP, migration.QUIESCE,
	)
	config := migrationflag.Config{
		Facade: facade,
		Model:  validUUID,
		Check:  isQuiesce,
	}
	worker, err := migrationflag.New(c.Context(), config)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(worker.Check(), tc.IsFalse)

	err = workertest.CheckKilled(c, worker)
	c.Check(err, tc.Equals, migrationflag.ErrChanged)
	checkCalls(c, stub, "Phase", "Watch", "Phase", "Phase")
}

func (*WorkerSuite) TestNoRelevantPhaseChange(c *tc.C) {
	stub := &testhelpers.Stub{}
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
	worker, err := migrationflag.New(c.Context(), config)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(worker.Check(), tc.IsFalse)

	workertest.CheckAlive(c, worker)
	workertest.CleanKill(c, worker)
	checkCalls(c, stub, "Phase", "Watch", "Phase", "Phase", "Phase")
}

func (*WorkerSuite) TestIsTerminal(c *tc.C) {
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
		c.Check(migrationflag.IsTerminal(t.phase), tc.Equals, t.expected,
			tc.Commentf("for %s", t.phase))
	}
}
