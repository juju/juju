// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationmaster_test

import (
	"github.com/juju/errors"
	"github.com/juju/juju/api"
	"github.com/juju/juju/migration"
	"github.com/juju/juju/worker/fortress"
	"github.com/juju/juju/worker/migrationmaster"
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

func (*ValidateSuite) TestMissingConn(c *gc.C) {
	config := validConfig()
	config.SourceConn = nil
	checkNotValid(c, config, "nil SourceConn not valid")
}

func (*ValidateSuite) TestMissingUploadBinaries(c *gc.C) {
	config := validConfig()
	config.UploadBinaries = nil
	checkNotValid(c, config, "nil UploadBinaries not valid")
}

func validConfig() migrationmaster.Config {
	return migrationmaster.Config{
		Guard:          struct{ fortress.Guard }{},
		Facade:         struct{ migrationmaster.Facade }{},
		SourceConn:     struct{ api.Connection }{},
		UploadBinaries: func(migration.UploadBinariesConfig) error { return nil },
	}
}

func checkNotValid(c *gc.C, config migrationmaster.Config, expect string) {
	check := func(err error) {
		c.Check(err, gc.ErrorMatches, expect)
		c.Check(err, jc.Satisfies, errors.IsNotValid)
	}

	err := config.Validate()
	check(err)

	worker, err := migrationmaster.New(config)
	c.Check(worker, gc.IsNil)
	check(err)
}
