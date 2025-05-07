// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationflag_test

import (
	"github.com/juju/tc"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
)

type ValidateSuite struct {
	testing.IsolationSuite
}

var _ = tc.Suite(&ValidateSuite{})

func (*ValidateSuite) TestValid(c *tc.C) {
	config := validConfig()
	err := config.Validate()
	c.Check(err, jc.ErrorIsNil)
}

func (*ValidateSuite) TestNilFacade(c *tc.C) {
	config := validConfig()
	config.Facade = nil
	checkNotValid(c, config, "nil Facade not valid")
}

func (*ValidateSuite) TestInvalidModel(c *tc.C) {
	config := validConfig()
	config.Model = "abcd-1234"
	checkNotValid(c, config, `Model "abcd-1234" not valid`)
}

func (*ValidateSuite) TestNilCheck(c *tc.C) {
	config := validConfig()
	config.Check = nil
	checkNotValid(c, config, "nil Check not valid")
}
