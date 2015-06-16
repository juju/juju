// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lease_test

import (
	_ "time"

	_ "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	_ "gopkg.in/mgo.v2/bson"

	_ "github.com/juju/juju/state/lease"
)

// ClientAssertSuite tests that AssertOp does what it should.
type ClientAssertSuite struct {
	FixtureSuite
	fix *Fixture
}

var _ = gc.Suite(&ClientAssertSuite{})

func (s *ClientAssertSuite) SetUpTest(c *gc.C) {
	s.FixtureSuite.SetUpTest(c)
	s.fix = s.EasyFixture(c)
}

func (s *ClientAssertSuite) TestPassesWhenLeaseHeld(c *gc.C) {
	c.Fatalf("not done")
}

func (s *ClientAssertSuite) TestPassesWhenLeaseStillHeldDespiteWriterChange(c *gc.C) {
	c.Fatalf("not done")
}

func (s *ClientAssertSuite) TestAbortsWhenLeaseVacant(c *gc.C) {
	c.Fatalf("not done")
}
