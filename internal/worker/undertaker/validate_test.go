// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package undertaker_test

import (
	"context"
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/worker/undertaker"
)

type ValidateSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&ValidateSuite{})

func (*ValidateSuite) TestNilFacade(c *gc.C) {
	config := validConfig(c)
	config.Facade = nil
	checkInvalid(c, config, "nil Facade not valid")
}

func (*ValidateSuite) TestNilLogger(c *gc.C) {
	config := validConfig(c)
	config.Logger = nil
	checkInvalid(c, config, "nil Logger not valid")
}

func (*ValidateSuite) TestNilNewCloudDestroyerFunc(c *gc.C) {
	config := validConfig(c)
	config.NewCloudDestroyerFunc = nil
	checkInvalid(c, config, "nil NewCloudDestroyerFunc not valid")
}

func (*ValidateSuite) TestNilClock(c *gc.C) {
	config := validConfig(c)
	config.Clock = nil
	checkInvalid(c, config, "nil Clock not valid")
}

func validConfig(c *gc.C) undertaker.Config {
	return undertaker.Config{
		Facade: &fakeFacade{},
		Logger: loggertesting.WrapCheckLog(c),
		Clock:  testclock.NewClock(time.Time{}),
		NewCloudDestroyerFunc: func(context.Context, environs.OpenParams, environs.CredentialInvalidator) (environs.CloudDestroyer, error) {
			return nil, nil
		},
	}
}

func checkInvalid(c *gc.C, config undertaker.Config, message string) {
	check := func(err error) {
		c.Check(err, jc.ErrorIs, errors.NotValid)
		c.Check(err, gc.ErrorMatches, message)
	}
	err := config.Validate()
	check(err)

	worker, err := undertaker.NewUndertaker(config)
	c.Check(worker, gc.IsNil)
	check(err)
}
