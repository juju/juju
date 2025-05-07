// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package credentialvalidator_test

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
	config := validConfig(c)
	err := config.Validate()
	c.Check(err, jc.ErrorIsNil)
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
