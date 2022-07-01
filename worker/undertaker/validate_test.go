// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package undertaker_test

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/v2/worker/undertaker"
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

func (*ValidateSuite) TestNilDestroyer(c *gc.C) {
	config := validConfig()
	config.Destroyer = nil
	checkInvalid(c, config, "nil Destroyer not valid")
}

func (*ValidateSuite) TestNilCredentialAPI(c *gc.C) {
	config := validConfig()
	config.CredentialAPI = nil
	checkInvalid(c, config, "nil CredentialAPI not valid")
}

func (*ValidateSuite) TestNilLogger(c *gc.C) {
	config := validConfig()
	config.Logger = nil
	checkInvalid(c, config, "nil Logger not valid")
}

func validConfig() undertaker.Config {
	return undertaker.Config{
		Facade:        &fakeFacade{},
		Destroyer:     &fakeEnviron{},
		CredentialAPI: &fakeCredentialAPI{},
		Logger:        &fakeLogger{},
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
