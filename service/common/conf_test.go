// Copyright 2015 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package common_test

import (
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/service/common"
)

type confSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&confSuite{})

func (*confSuite) TestValidateOkay(c *gc.C) {
	conf := common.Conf{
		Desc: "some service",
		Cmd:  "<do something>",
	}
	err := conf.Validate()

	c.Check(err, jc.ErrorIsNil)
}

func (*confSuite) TestValidateMissingDesc(c *gc.C) {
	conf := common.Conf{
		Cmd: "<do something>",
	}
	err := conf.Validate()

	c.Check(err, gc.ErrorMatches, ".*missing Desc.*")
}

func (*confSuite) TestValidateMissingCmd(c *gc.C) {
	conf := common.Conf{
		Desc: "some service",
	}
	err := conf.Validate()

	c.Check(err, gc.ErrorMatches, ".*missing Cmd.*")
}
