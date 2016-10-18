// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lifeflag_test

import (
	"github.com/juju/errors"
	"github.com/juju/juju/worker/lifeflag"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

type ValidateSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&ValidateSuite{})

func (*ValidateSuite) TestValidConfig(c *gc.C) {
	config := validConfig()
	err := config.Validate()
	c.Check(err, jc.ErrorIsNil)
}

func (*ValidateSuite) TestNilFacade(c *gc.C) {
	config := validConfig()
	config.Facade = nil
	checkInvalid(c, config, "nil Facade not valid")
}

func (*ValidateSuite) TestNilEntity(c *gc.C) {
	config := validConfig()
	config.Entity = nil
	checkInvalid(c, config, "nil Entity not valid")
}

func (*ValidateSuite) TestNilResult(c *gc.C) {
	config := validConfig()
	config.Result = nil
	checkInvalid(c, config, "nil Result not valid")
}

func checkInvalid(c *gc.C, config lifeflag.Config, message string) {
	check := func(err error) {
		c.Check(err, jc.Satisfies, errors.IsNotValid)
		c.Check(err, gc.ErrorMatches, message)
	}
	err := config.Validate()
	check(err)

	worker, err := lifeflag.New(config)
	c.Check(worker, gc.IsNil)
	check(err)
}

func validConfig() lifeflag.Config {
	return lifeflag.Config{
		Facade: struct{ lifeflag.Facade }{},
		Entity: testEntity,
		Result: explode,
	}
}
