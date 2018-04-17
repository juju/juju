// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationflag_test

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
	config := validConfig()
	err := config.Validate()
	c.Check(err, jc.ErrorIsNil)
}

func (*ValidateSuite) TestNilFacade(c *gc.C) {
	config := validConfig()
	config.Facade = nil
	checkNotValid(c, config, "nil Facade not valid")
}

func (*ValidateSuite) TestInvalidModel(c *gc.C) {
	config := validConfig()
	config.Model = "abcd-1234"
	checkNotValid(c, config, `Model "abcd-1234" not valid`)
}

func (*ValidateSuite) TestNilCheck(c *gc.C) {
	config := validConfig()
	config.Check = nil
	checkNotValid(c, config, "nil Check not valid")
}
