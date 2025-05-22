// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationminion_test

import (
	"context"
	"testing"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/tc"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api"
	"github.com/juju/juju/api/base"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/worker/fortress"
	"github.com/juju/juju/internal/worker/migrationminion"
)

type ValidateSuite struct {
	testhelpers.IsolationSuite
}

func TestValidateSuite(t *testing.T) {
	tc.Run(t, &ValidateSuite{})
}

func (*ValidateSuite) TestValid(c *tc.C) {
	err := validConfig(c).Validate()
	c.Check(err, tc.ErrorIsNil)
}

func (*ValidateSuite) TestMissingAgent(c *tc.C) {
	config := validConfig(c)
	config.Agent = nil
	checkNotValid(c, config, "nil Agent not valid")
}

func (*ValidateSuite) TestMissingFacade(c *tc.C) {
	config := validConfig(c)
	config.Facade = nil
	checkNotValid(c, config, "nil Facade not valid")
}

func (*ValidateSuite) TestMissingClock(c *tc.C) {
	config := validConfig(c)
	config.Clock = nil
	checkNotValid(c, config, "nil Clock not valid")
}

func (*ValidateSuite) TestMissingGuard(c *tc.C) {
	config := validConfig(c)
	config.Guard = nil
	checkNotValid(c, config, "nil Guard not valid")
}

func (*ValidateSuite) TestMissingAPIOpen(c *tc.C) {
	config := validConfig(c)
	config.APIOpen = nil
	checkNotValid(c, config, "nil APIOpen not valid")
}

func (*ValidateSuite) TestMissingValidateMigration(c *tc.C) {
	config := validConfig(c)
	config.ValidateMigration = nil
	checkNotValid(c, config, "nil ValidateMigration not valid")
}

func (*ValidateSuite) TestMissingLogger(c *tc.C) {
	config := validConfig(c)
	config.Logger = nil
	checkNotValid(c, config, "nil Logger not valid")
}

func validConfig(c *tc.C) migrationminion.Config {
	return migrationminion.Config{
		Agent:             struct{ agent.Agent }{},
		Guard:             struct{ fortress.Guard }{},
		Facade:            struct{ migrationminion.Facade }{},
		Clock:             struct{ clock.Clock }{},
		APIOpen:           func(context.Context, *api.Info, api.DialOpts) (api.Connection, error) { return nil, nil },
		ValidateMigration: func(context.Context, base.APICaller) error { return nil },
		Logger:            loggertesting.WrapCheckLog(c),
	}
}

func checkNotValid(c *tc.C, config migrationminion.Config, expect string) {
	check := func(err error) {
		c.Check(err, tc.ErrorMatches, expect)
		c.Check(err, tc.ErrorIs, errors.NotValid)
	}

	err := config.Validate()
	check(err)

	worker, err := migrationminion.New(config)
	c.Check(worker, tc.IsNil)
	check(err)
}
