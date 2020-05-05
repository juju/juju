// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resumer_test

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v2/workertest"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/worker/resumer"
)

type ConfigSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&ConfigSuite{})

func (*ConfigSuite) TestValid(c *gc.C) {
	config := validConfig()
	err := config.Validate()
	c.Check(err, jc.ErrorIsNil)
}

func (*ConfigSuite) TestNilFacade(c *gc.C) {
	config := validConfig()
	config.Facade = nil
	checkInvalid(c, config, "nil Facade not valid")
}

func (*ConfigSuite) TestNilClock(c *gc.C) {
	config := validConfig()
	config.Clock = nil
	checkInvalid(c, config, "nil Clock not valid")
}

func (*ConfigSuite) TestZeroInterval(c *gc.C) {
	config := validConfig()
	config.Interval = 0
	checkInvalid(c, config, "non-positive Interval not valid")
}

func (*ConfigSuite) TestNegativeInterval(c *gc.C) {
	config := validConfig()
	config.Interval = -time.Minute
	checkInvalid(c, config, "non-positive Interval not valid")
}

func validConfig() resumer.Config {
	return resumer.Config{
		Facade:   &fakeFacade{},
		Clock:    &fakeClock{},
		Interval: time.Minute,
	}
}

func checkInvalid(c *gc.C, config resumer.Config, match string) {
	check := func(err error) {
		c.Check(err, jc.Satisfies, errors.IsNotValid)
		c.Check(err, gc.ErrorMatches, match)
	}
	check(config.Validate())

	worker, err := resumer.NewResumer(config)
	workertest.CheckNilOrKill(c, worker)
	check(err)
}
