// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migration_test

import (
	stdtesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/core/migration"
	coretesting "github.com/juju/juju/internal/testing"
)

type PhaseSuite struct {
	coretesting.BaseSuite
}

func TestPhaseSuite(t *stdtesting.T) {
	tc.Run(t, new(PhaseSuite))
}

func (s *PhaseSuite) TestUNKNOWN(c *tc.C) {
	// 0 should be UNKNOWN to guard against uninitialised struct
	// fields.
	c.Check(migration.Phase(0), tc.Equals, migration.UNKNOWN)
}

func (s *PhaseSuite) TestStringValid(c *tc.C) {
	c.Check(migration.IMPORT.String(), tc.Equals, "IMPORT")
	c.Check(migration.UNKNOWN.String(), tc.Equals, "UNKNOWN")
	c.Check(migration.ABORT.String(), tc.Equals, "ABORT")
}

func (s *PhaseSuite) TestInvalid(c *tc.C) {
	c.Check(migration.Phase(-1).String(), tc.Equals, "UNKNOWN")
	c.Check(migration.Phase(9999).String(), tc.Equals, "UNKNOWN")
}

func (s *PhaseSuite) TestParseValid(c *tc.C) {
	phase, ok := migration.ParsePhase("REAP")
	c.Check(phase, tc.Equals, migration.REAP)
	c.Check(ok, tc.IsTrue)
}

func (s *PhaseSuite) TestParseInvalid(c *tc.C) {
	phase, ok := migration.ParsePhase("foo")
	c.Check(phase, tc.Equals, migration.UNKNOWN)
	c.Check(ok, tc.IsFalse)
}

func (s *PhaseSuite) TestIsTerminal(c *tc.C) {
	c.Check(migration.QUIESCE.IsTerminal(), tc.IsFalse)
	c.Check(migration.SUCCESS.IsTerminal(), tc.IsFalse)
	c.Check(migration.ABORT.IsTerminal(), tc.IsFalse)
	c.Check(migration.ABORTDONE.IsTerminal(), tc.IsTrue)
	c.Check(migration.REAPFAILED.IsTerminal(), tc.IsTrue)
	c.Check(migration.DONE.IsTerminal(), tc.IsTrue)
}

func (s *PhaseSuite) TestIsRunning(c *tc.C) {
	c.Check(migration.UNKNOWN.IsRunning(), tc.IsFalse)
	c.Check(migration.NONE.IsRunning(), tc.IsFalse)

	c.Check(migration.QUIESCE.IsRunning(), tc.IsTrue)
	c.Check(migration.IMPORT.IsRunning(), tc.IsTrue)
	c.Check(migration.PROCESSRELATIONS.IsRunning(), tc.IsTrue)
	c.Check(migration.SUCCESS.IsRunning(), tc.IsTrue)

	c.Check(migration.LOGTRANSFER.IsRunning(), tc.IsFalse)
	c.Check(migration.REAP.IsRunning(), tc.IsFalse)
	c.Check(migration.REAPFAILED.IsRunning(), tc.IsFalse)
	c.Check(migration.DONE.IsRunning(), tc.IsFalse)
	c.Check(migration.ABORT.IsRunning(), tc.IsFalse)
	c.Check(migration.ABORTDONE.IsRunning(), tc.IsFalse)
}

func (s *PhaseSuite) TestCanTransitionTo(c *tc.C) {
	c.Check(migration.QUIESCE.CanTransitionTo(migration.SUCCESS), tc.IsFalse)
	c.Check(migration.QUIESCE.CanTransitionTo(migration.ABORT), tc.IsTrue)
	c.Check(migration.QUIESCE.CanTransitionTo(migration.IMPORT), tc.IsTrue)
	c.Check(migration.QUIESCE.CanTransitionTo(migration.PROCESSRELATIONS), tc.IsFalse)
	c.Check(migration.QUIESCE.CanTransitionTo(migration.Phase(-1)), tc.IsFalse)
	c.Check(migration.ABORT.CanTransitionTo(migration.QUIESCE), tc.IsFalse)
}
