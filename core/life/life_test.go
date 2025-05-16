// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package life_test

import (
	stdtesting "testing"

	"github.com/juju/tc"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/internal/testhelpers"
)

type LifeSuite struct {
	testhelpers.IsolationSuite
}

func TestLifeSuite(t *stdtesting.T) { tc.Run(t, &LifeSuite{}) }
func (*LifeSuite) TestValidateValid(c *tc.C) {
	for i, test := range []life.Value{
		life.Alive, life.Dying, life.Dead,
	} {
		c.Logf("test %d: %s", i, test)
		err := test.Validate()
		c.Check(err, tc.ErrorIsNil)
	}
}

func (*LifeSuite) TestValidateInvalid(c *tc.C) {
	for i, test := range []life.Value{
		"", "bad", "resurrected",
		" alive", "alive ", "Alive",
	} {
		c.Logf("test %d: %s", i, test)
		err := test.Validate()
		c.Check(err, tc.ErrorIs, coreerrors.NotValid)
		c.Check(err, tc.ErrorMatches, `life value ".*" not valid`)
	}
}

func (*LifeSuite) TestIsAliveSuccess(c *tc.C) {
	c.Check(life.IsAlive(life.Alive), tc.IsTrue)
}

func (*LifeSuite) TestIsAliveFailure(c *tc.C) {
	for i, test := range []life.Value{
		life.Dying, life.Dead, "", "bad", "ALIVE",
	} {
		c.Logf("test %d: %s", i, test)
		c.Check(life.IsAlive(test), tc.IsFalse)
	}
}

func (*LifeSuite) TestIsNotAliveFailure(c *tc.C) {
	c.Check(life.IsNotAlive(life.Alive), tc.IsFalse)
}

func (*LifeSuite) TestIsNotAliveSuccess(c *tc.C) {
	for i, test := range []life.Value{
		life.Dying, life.Dead, "", "bad", "ALIVE",
	} {
		c.Logf("test %d: %s", i, test)
		c.Check(life.IsNotAlive(test), tc.IsTrue)
	}
}

func (*LifeSuite) TestIsNotDeadFailure(c *tc.C) {
	c.Check(life.IsNotDead(life.Dead), tc.IsFalse)
}

func (*LifeSuite) TestIsNotDeadSuccess(c *tc.C) {
	for i, test := range []life.Value{
		life.Alive, life.Dying, "", "bad", "DEAD",
	} {
		c.Logf("test %d: %s", i, test)
		c.Check(life.IsNotDead(test), tc.IsTrue)
	}
}

func (*LifeSuite) TestIsDeadSuccess(c *tc.C) {
	c.Check(life.IsDead(life.Dead), tc.IsTrue)
}

func (*LifeSuite) TestIsDeadFailure(c *tc.C) {
	for i, test := range []life.Value{
		life.Alive, life.Dying, "", "bad", "DEAD",
	} {
		c.Logf("test %d: %s", i, test)
		c.Check(life.IsDead(test), tc.IsFalse)
	}
}
