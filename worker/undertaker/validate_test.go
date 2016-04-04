// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package undertaker_test

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/worker/undertaker"
)

type ValidateSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&ValidateSuite{})

func (*ValidateSuite) TestNilFacade(c *gc.C) {
	config := validConfig()
	config.Facade = nil
	checkInvalid(c, config, "nil Facade not valid")
}

func (*ValidateSuite) TestNilEnviron(c *gc.C) {
	config := validConfig()
	config.Environ = nil
	checkInvalid(c, config, "nil Environ not valid")
}

func (*ValidateSuite) TestNilClock(c *gc.C) {
	config := validConfig()
	config.Clock = nil
	checkInvalid(c, config, "nil Clock not valid")
}

func (*ValidateSuite) TestZeroDelay(c *gc.C) {
	config := validConfig()
	config.RemoveDelay = 0
	checkInvalid(c, config, "non-positive RemoveDelay not valid")
}

func (*ValidateSuite) TestNegativeDelay(c *gc.C) {
	config := validConfig()
	config.RemoveDelay = -time.Second
	checkInvalid(c, config, "non-positive RemoveDelay not valid")
}

func validConfig() undertaker.Config {
	return undertaker.Config{
		Facade:      &fakeFacade{},
		Environ:     &fakeEnviron{},
		Clock:       &fakeClock{},
		RemoveDelay: time.Hour,
	}
}

func checkInvalid(c *gc.C, config undertaker.Config, message string) {
	check := func(err error) {
		c.Check(err, jc.Satisfies, errors.IsNotValid)
		c.Check(err, gc.ErrorMatches, message)
	}
	err := config.Validate()
	check(err)

	worker, err := undertaker.NewUndertaker(config)
	c.Check(worker, gc.IsNil)
	check(err)
}
