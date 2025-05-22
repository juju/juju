// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationmaster_test

import (
	"context"
	"testing"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/tc"

	"github.com/juju/juju/api"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/internal/migration"
	"github.com/juju/juju/internal/testhelpers"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/worker/fortress"
	"github.com/juju/juju/internal/worker/migrationmaster"
)

type ValidateSuite struct {
	testhelpers.IsolationSuite
}

func TestValidateSuite(t *testing.T) {
	tc.Run(t, &ValidateSuite{})
}

func (*ValidateSuite) TestValid(c *tc.C) {
	err := validConfig().Validate()
	c.Check(err, tc.ErrorIsNil)
}

func (*ValidateSuite) TestMissingModelUUID(c *tc.C) {
	config := validConfig()
	config.ModelUUID = ""
	checkNotValid(c, config, `model UUID "" not valid`)
}

func (*ValidateSuite) TestMissingGuard(c *tc.C) {
	config := validConfig()
	config.Guard = nil
	checkNotValid(c, config, "nil Guard not valid")
}

func (*ValidateSuite) TestMissingFacade(c *tc.C) {
	config := validConfig()
	config.Facade = nil
	checkNotValid(c, config, "nil Facade not valid")
}

func (*ValidateSuite) TestMissingAPIOpen(c *tc.C) {
	config := validConfig()
	config.APIOpen = nil
	checkNotValid(c, config, "nil APIOpen not valid")
}

func (*ValidateSuite) TestMissingUploadBinaries(c *tc.C) {
	config := validConfig()
	config.UploadBinaries = nil
	checkNotValid(c, config, "nil UploadBinaries not valid")
}

func (*ValidateSuite) TestMissingCharmService(c *tc.C) {
	config := validConfig()
	config.CharmService = nil
	checkNotValid(c, config, "nil CharmService not valid")
}

func (*ValidateSuite) TestMissingAgentBinaryStore(c *tc.C) {
	config := validConfig()
	config.AgentBinaryStore = nil
	checkNotValid(c, config, "nil AgentBinaryStore not valid")
}

func (*ValidateSuite) TestMissingClock(c *tc.C) {
	config := validConfig()
	config.Clock = nil
	checkNotValid(c, config, "nil Clock not valid")
}

func validConfig() migrationmaster.Config {
	return migrationmaster.Config{
		ModelUUID:        coretesting.ModelTag.Id(),
		Guard:            struct{ fortress.Guard }{},
		Facade:           struct{ migrationmaster.Facade }{},
		APIOpen:          func(context.Context, *api.Info, api.DialOpts) (api.Connection, error) { return nil, nil },
		UploadBinaries:   func(context.Context, migration.UploadBinariesConfig, logger.Logger) error { return nil },
		CharmService:     struct{ migrationmaster.CharmService }{},
		AgentBinaryStore: struct{ migration.AgentBinaryStore }{},
		Clock:            struct{ clock.Clock }{},
	}
}

func checkNotValid(c *tc.C, config migrationmaster.Config, expect string) {
	check := func(err error) {
		c.Check(err, tc.ErrorMatches, expect)
		c.Check(err, tc.ErrorIs, errors.NotValid)
	}

	err := config.Validate()
	check(err)

	worker, err := migrationmaster.New(config)
	c.Check(worker, tc.IsNil)
	check(err)
}
