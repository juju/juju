// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	migration "github.com/juju/juju/core/modelmigration"
	coretesting "github.com/juju/juju/testing"
)

type PhaseSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(new(PhaseSuite))

func (s *PhaseSuite) TestUNKNOWN(c *gc.C) {
	// 0 should be UNKNOWN to guard against uninitialised struct
	// fields.
	c.Check(migration.Phase(0), gc.Equals, migration.UNKNOWN)
}

func (s *PhaseSuite) TestStringValid(c *gc.C) {
	c.Check(migration.PRECHECK.String(), gc.Equals, "PRECHECK")
	c.Check(migration.UNKNOWN.String(), gc.Equals, "UNKNOWN")
	c.Check(migration.ABORT.String(), gc.Equals, "ABORT")
}

func (s *PhaseSuite) TestInvalid(c *gc.C) {
	c.Check(migration.Phase(-1).String(), gc.Equals, "UNKNOWN")
	c.Check(migration.Phase(9999).String(), gc.Equals, "UNKNOWN")
}

func (s *PhaseSuite) TestParseValid(c *gc.C) {
	phase, ok := migration.ParsePhase("REAP")
	c.Check(phase, gc.Equals, migration.REAP)
	c.Check(ok, jc.IsTrue)
}

func (s *PhaseSuite) TestParseInvalid(c *gc.C) {
	phase, ok := migration.ParsePhase("foo")
	c.Check(phase, gc.Equals, migration.UNKNOWN)
	c.Check(ok, jc.IsFalse)
}

func (s *PhaseSuite) TestIsTerminal(c *gc.C) {
	c.Check(migration.QUIESCE.IsTerminal(), jc.IsFalse)
	c.Check(migration.SUCCESS.IsTerminal(), jc.IsFalse)
	c.Check(migration.ABORT.IsTerminal(), jc.IsTrue)
	c.Check(migration.REAPFAILED.IsTerminal(), jc.IsTrue)
	c.Check(migration.DONE.IsTerminal(), jc.IsTrue)
}

func (s *PhaseSuite) TestCanTransitionTo(c *gc.C) {
	c.Check(migration.QUIESCE.CanTransitionTo(migration.SUCCESS), jc.IsFalse)
	c.Check(migration.QUIESCE.CanTransitionTo(migration.ABORT), jc.IsTrue)
	c.Check(migration.QUIESCE.CanTransitionTo(migration.READONLY), jc.IsTrue)
	c.Check(migration.QUIESCE.CanTransitionTo(migration.Phase(-1)), jc.IsFalse)

	c.Check(migration.ABORT.CanTransitionTo(migration.QUIESCE), jc.IsFalse)
}
