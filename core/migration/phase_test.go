// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migration_test

import (
	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"

	"github.com/juju/juju/core/migration"
	coretesting "github.com/juju/juju/internal/testing"
)

type PhaseSuite struct {
	coretesting.BaseSuite
}

var _ = tc.Suite(new(PhaseSuite))

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
	c.Check(ok, jc.IsTrue)
}

func (s *PhaseSuite) TestParseInvalid(c *tc.C) {
	phase, ok := migration.ParsePhase("foo")
	c.Check(phase, tc.Equals, migration.UNKNOWN)
	c.Check(ok, jc.IsFalse)
}

func (s *PhaseSuite) TestIsTerminal(c *tc.C) {
	c.Check(migration.QUIESCE.IsTerminal(), jc.IsFalse)
	c.Check(migration.SUCCESS.IsTerminal(), jc.IsFalse)
	c.Check(migration.ABORT.IsTerminal(), jc.IsFalse)
	c.Check(migration.ABORTDONE.IsTerminal(), jc.IsTrue)
	c.Check(migration.REAPFAILED.IsTerminal(), jc.IsTrue)
	c.Check(migration.DONE.IsTerminal(), jc.IsTrue)
}

func (s *PhaseSuite) TestIsRunning(c *tc.C) {
	c.Check(migration.UNKNOWN.IsRunning(), jc.IsFalse)
	c.Check(migration.NONE.IsRunning(), jc.IsFalse)

	c.Check(migration.QUIESCE.IsRunning(), jc.IsTrue)
	c.Check(migration.IMPORT.IsRunning(), jc.IsTrue)
	c.Check(migration.PROCESSRELATIONS.IsRunning(), jc.IsTrue)
	c.Check(migration.SUCCESS.IsRunning(), jc.IsTrue)

	c.Check(migration.LOGTRANSFER.IsRunning(), jc.IsFalse)
	c.Check(migration.REAP.IsRunning(), jc.IsFalse)
	c.Check(migration.REAPFAILED.IsRunning(), jc.IsFalse)
	c.Check(migration.DONE.IsRunning(), jc.IsFalse)
	c.Check(migration.ABORT.IsRunning(), jc.IsFalse)
	c.Check(migration.ABORTDONE.IsRunning(), jc.IsFalse)
}

func (s *PhaseSuite) TestCanTransitionTo(c *tc.C) {
	c.Check(migration.QUIESCE.CanTransitionTo(migration.SUCCESS), jc.IsFalse)
	c.Check(migration.QUIESCE.CanTransitionTo(migration.ABORT), jc.IsTrue)
	c.Check(migration.QUIESCE.CanTransitionTo(migration.IMPORT), jc.IsTrue)
	c.Check(migration.QUIESCE.CanTransitionTo(migration.PROCESSRELATIONS), jc.IsFalse)
	c.Check(migration.QUIESCE.CanTransitionTo(migration.Phase(-1)), jc.IsFalse)
	c.Check(migration.ABORT.CanTransitionTo(migration.QUIESCE), jc.IsFalse)
}
