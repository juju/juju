// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/service/common"
)

type serviceSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&serviceSuite{})

func (*serviceSuite) TestNoConfMissing(c *gc.C) {
	service := common.Service{
		Name: "a-service",
	}
	noConf := service.NoConf()

	c.Check(noConf, jc.IsTrue)
}

func (*serviceSuite) TestNoConfEmpty(c *gc.C) {
	service := common.Service{
		Name: "a-service",
		Conf: common.Conf{},
	}
	noConf := service.NoConf()

	c.Check(noConf, jc.IsTrue)
}

func (*serviceSuite) TestNoConfFalse(c *gc.C) {
	service := common.Service{
		Name: "a-service",
		Conf: common.Conf{
			Desc:      "some service",
			ExecStart: "/path/to/some-command x y z",
		},
	}
	noConf := service.NoConf()

	c.Check(noConf, jc.IsFalse)
}

func (*serviceSuite) TestValidateOkay(c *gc.C) {
	service := common.Service{
		Name: "a-service",
		Conf: common.Conf{
			Desc:      "some service",
			ExecStart: "/path/to/some-command x y z",
		},
	}
	err := service.Validate(renderer)

	c.Check(err, jc.ErrorIsNil)
}

func (*serviceSuite) TestValidateMissingName(c *gc.C) {
	service := common.Service{
		Conf: common.Conf{
			Desc:      "some service",
			ExecStart: "/path/to/some-command x y z",
		},
	}
	err := service.Validate(renderer)

	c.Check(err, gc.ErrorMatches, ".*missing Name.*")
}

func (*serviceSuite) TestValidateMissingDesc(c *gc.C) {
	service := common.Service{
		Name: "a-service",
		Conf: common.Conf{
			ExecStart: "/path/to/some-command x y z",
		},
	}
	err := service.Validate(renderer)

	c.Check(err, gc.ErrorMatches, ".*missing Desc.*")
}

func (*serviceSuite) TestValidateMissingExecStart(c *gc.C) {
	service := common.Service{
		Name: "a-service",
		Conf: common.Conf{
			Desc: "some service",
		},
	}
	err := service.Validate(renderer)

	c.Check(err, gc.ErrorMatches, ".*missing ExecStart.*")
}
