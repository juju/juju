// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package life_test

import (
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/life"
)

type LifeSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&LifeSuite{})

func (*LifeSuite) TestValidateValid(c *gc.C) {
	for i, test := range []life.Value{
		life.Alive, life.Dying, life.Dead,
	} {
		c.Logf("test %d: %s", i, test)
		err := test.Validate()
		c.Check(err, jc.ErrorIsNil)
	}
}

func (*LifeSuite) TestValidateInvalid(c *gc.C) {
	for i, test := range []life.Value{
		"", "bad", "resurrected",
		" alive", "alive ", "Alive",
	} {
		c.Logf("test %d: %s", i, test)
		err := test.Validate()
		c.Check(err, jc.ErrorIs, coreerrors.NotValid)
		c.Check(err, gc.ErrorMatches, `life value ".*" not valid`)
	}
}

func (*LifeSuite) TestIsAliveSuccess(c *gc.C) {
	c.Check(life.IsAlive(life.Alive), jc.IsTrue)
}

func (*LifeSuite) TestIsAliveFailure(c *gc.C) {
	for i, test := range []life.Value{
		life.Dying, life.Dead, "", "bad", "ALIVE",
	} {
		c.Logf("test %d: %s", i, test)
		c.Check(life.IsAlive(test), jc.IsFalse)
	}
}

func (*LifeSuite) TestIsNotAliveFailure(c *gc.C) {
	c.Check(life.IsNotAlive(life.Alive), jc.IsFalse)
}

func (*LifeSuite) TestIsNotAliveSuccess(c *gc.C) {
	for i, test := range []life.Value{
		life.Dying, life.Dead, "", "bad", "ALIVE",
	} {
		c.Logf("test %d: %s", i, test)
		c.Check(life.IsNotAlive(test), jc.IsTrue)
	}
}

func (*LifeSuite) TestIsNotDeadFailure(c *gc.C) {
	c.Check(life.IsNotDead(life.Dead), jc.IsFalse)
}

func (*LifeSuite) TestIsNotDeadSuccess(c *gc.C) {
	for i, test := range []life.Value{
		life.Alive, life.Dying, "", "bad", "DEAD",
	} {
		c.Logf("test %d: %s", i, test)
		c.Check(life.IsNotDead(test), jc.IsTrue)
	}
}

func (*LifeSuite) TestIsDeadSuccess(c *gc.C) {
	c.Check(life.IsDead(life.Dead), jc.IsTrue)
}

func (*LifeSuite) TestIsDeadFailure(c *gc.C) {
	for i, test := range []life.Value{
		life.Alive, life.Dying, "", "bad", "DEAD",
	} {
		c.Logf("test %d: %s", i, test)
		c.Check(life.IsDead(test), jc.IsFalse)
	}
}
