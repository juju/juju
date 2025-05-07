// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package credentialvalidator_test

import (
	"github.com/juju/tc"

	"github.com/juju/juju/internal/testhelpers"
)

type ValidateSuite struct {
	testhelpers.IsolationSuite
}

var _ = tc.Suite(&ValidateSuite{})

func (*ValidateSuite) TestValid(c *tc.C) {
	config := validConfig(c)
	err := config.Validate()
	c.Check(err, tc.ErrorIsNil)
}

func (*ValidateSuite) TestNilFacade(c *tc.C) {
	config := validConfig(c)
	config.Facade = nil
	checkNotValid(c, config, "nil Facade not valid")
}

func (*ValidateSuite) TestNilLogger(c *tc.C) {
	config := validConfig(c)
	config.Logger = nil
	checkNotValid(c, config, "nil Logger not valid")
}
