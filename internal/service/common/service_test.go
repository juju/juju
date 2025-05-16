// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	stdtesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/internal/service/common"
	"github.com/juju/juju/internal/testhelpers"
)

type serviceSuite struct {
	testhelpers.IsolationSuite
}

func TestServiceSuite(t *stdtesting.T) { tc.Run(t, &serviceSuite{}) }
func (*serviceSuite) TestNoConfMissing(c *tc.C) {
	service := common.Service{
		Name: "a-application",
	}
	noConf := service.NoConf()

	c.Check(noConf, tc.IsTrue)
}

func (*serviceSuite) TestNoConfEmpty(c *tc.C) {
	service := common.Service{
		Name: "a-application",
		Conf: common.Conf{},
	}
	noConf := service.NoConf()

	c.Check(noConf, tc.IsTrue)
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

	c.Check(noConf, tc.IsFalse)
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

	c.Check(err, tc.ErrorIsNil)
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
