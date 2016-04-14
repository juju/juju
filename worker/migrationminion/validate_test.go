// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationminion_test

import (
	"github.com/juju/errors"
	"github.com/juju/juju/agent"
	"github.com/juju/juju/worker/fortress"
	"github.com/juju/juju/worker/migrationminion"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

type ValidateSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&ValidateSuite{})

func (*ValidateSuite) TestValid(c *gc.C) {
	err := validConfig().Validate()
	c.Check(err, jc.ErrorIsNil)
}

func (*ValidateSuite) TestMissingAgent(c *gc.C) {
	config := validConfig()
	config.Agent = nil
	checkNotValid(c, config, "nil Agent not valid")
}

func (*ValidateSuite) TestMissingGuard(c *gc.C) {
	config := validConfig()
	config.Guard = nil
	checkNotValid(c, config, "nil Guard not valid")
}

func (*ValidateSuite) TestMissingFacade(c *gc.C) {
	config := validConfig()
	config.Facade = nil
	checkNotValid(c, config, "nil Facade not valid")
}

func validConfig() migrationminion.Config {
	return migrationminion.Config{
		Agent:  struct{ agent.Agent }{},
		Guard:  struct{ fortress.Guard }{},
		Facade: struct{ migrationminion.Facade }{},
	}
}

func checkNotValid(c *gc.C, config migrationminion.Config, expect string) {
	check := func(err error) {
		c.Check(err, gc.ErrorMatches, expect)
		c.Check(err, jc.Satisfies, errors.IsNotValid)
	}

	err := config.Validate()
	check(err)

	worker, err := migrationminion.New(config)
	c.Check(worker, gc.IsNil)
	check(err)
}
