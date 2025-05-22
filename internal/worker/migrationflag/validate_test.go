// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationflag_test

import (
	stdtesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/internal/testhelpers"
)

type ValidateSuite struct {
	testhelpers.IsolationSuite
}

func TestValidateSuite(t *stdtesting.T) {
	tc.Run(t, &ValidateSuite{})
}

func (*ValidateSuite) TestValid(c *tc.C) {
	config := validConfig()
	err := config.Validate()
	c.Check(err, tc.ErrorIsNil)
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
