// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lifeflag_test

import (
	stdtesting "testing"

	"github.com/juju/errors"
	"github.com/juju/tc"

	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/worker/lifeflag"
)

type ValidateSuite struct {
	testhelpers.IsolationSuite
}

func TestValidateSuite(t *stdtesting.T) {
	tc.Run(t, &ValidateSuite{})
}

func (*ValidateSuite) TestValidConfig(c *tc.C) {
	config := validConfig()
	err := config.Validate()
	c.Check(err, tc.ErrorIsNil)
}

func (*ValidateSuite) TestNilFacade(c *tc.C) {
	config := validConfig()
	config.Facade = nil
	checkInvalid(c, config, "nil Facade not valid")
}

func (*ValidateSuite) TestNilEntity(c *tc.C) {
	config := validConfig()
	config.Entity = nil
	checkInvalid(c, config, "nil Entity not valid")
}

func (*ValidateSuite) TestNilResult(c *tc.C) {
	config := validConfig()
	config.Result = nil
	checkInvalid(c, config, "nil Result not valid")
}

func checkInvalid(c *tc.C, config lifeflag.Config, message string) {
	check := func(err error) {
		c.Check(err, tc.ErrorIs, errors.NotValid)
		c.Check(err, tc.ErrorMatches, message)
	}
	err := config.Validate()
	check(err)

	worker, err := lifeflag.New(c.Context(), config)
	c.Check(worker, tc.IsNil)
	check(err)
}

func validConfig() lifeflag.Config {
	return lifeflag.Config{
		Facade: struct{ lifeflag.Facade }{},
		Entity: testEntity,
		Result: explode,
	}
}
