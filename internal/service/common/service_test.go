// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"github.com/juju/tc"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"

	"github.com/juju/juju/internal/service/common"
)

type serviceSuite struct {
	testing.IsolationSuite
}

var _ = tc.Suite(&serviceSuite{})

func (*serviceSuite) TestNoConfMissing(c *tc.C) {
	service := common.Service{
		Name: "a-application",
	}
	noConf := service.NoConf()

	c.Check(noConf, jc.IsTrue)
}

func (*serviceSuite) TestNoConfEmpty(c *tc.C) {
	service := common.Service{
		Name: "a-application",
		Conf: common.Conf{},
	}
	noConf := service.NoConf()

	c.Check(noConf, jc.IsTrue)
}

func (*serviceSuite) TestNoConfFalse(c *tc.C) {
	service := common.Service{
		Name: "a-application",
		Conf: common.Conf{
			Desc:      "some service",
			ExecStart: "/path/to/some-command x y z",
		},
	}
	noConf := service.NoConf()

	c.Check(noConf, jc.IsFalse)
}

func (*serviceSuite) TestValidateOkay(c *tc.C) {
	service := common.Service{
		Name: "a-application",
		Conf: common.Conf{
			Desc:      "some service",
			ExecStart: "/path/to/some-command x y z",
		},
	}
	err := service.Validate(renderer)

	c.Check(err, jc.ErrorIsNil)
}

func (*serviceSuite) TestValidateMissingName(c *tc.C) {
	service := common.Service{
		Conf: common.Conf{
			Desc:      "some service",
			ExecStart: "/path/to/some-command x y z",
		},
	}
	err := service.Validate(renderer)

	c.Check(err, tc.ErrorMatches, ".*missing Name.*")
}

func (*serviceSuite) TestValidateMissingDesc(c *tc.C) {
	service := common.Service{
		Name: "a-application",
		Conf: common.Conf{
			ExecStart: "/path/to/some-command x y z",
		},
	}
	err := service.Validate(renderer)

	c.Check(err, tc.ErrorMatches, ".*missing Desc.*")
}

func (*serviceSuite) TestValidateMissingExecStart(c *tc.C) {
	service := common.Service{
		Name: "a-application",
		Conf: common.Conf{
			Desc: "some service",
		},
	}
	err := service.Validate(renderer)

	c.Check(err, tc.ErrorMatches, ".*missing ExecStart.*")
}
