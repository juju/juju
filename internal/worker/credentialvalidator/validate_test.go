// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package credentialvalidator_test

import (
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

type ValidateSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&ValidateSuite{})

func (*ValidateSuite) TestValid(c *gc.C) {
	config := validConfig(c)
	err := config.Validate()
	c.Check(err, jc.ErrorIsNil)
}

func (*ValidateSuite) TestNilFacade(c *gc.C) {
	config := validConfig(c)
	config.Facade = nil
	checkNotValid(c, config, "nil Facade not valid")
}

func (*ValidateSuite) TestNilLogger(c *gc.C) {
	config := validConfig(c)
	config.Logger = nil
	checkNotValid(c, config, "nil Logger not valid")
}
