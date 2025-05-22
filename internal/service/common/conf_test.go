// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"testing"

	"github.com/juju/tc"
	"github.com/juju/utils/v4/shell"

	"github.com/juju/juju/internal/service/common"
	"github.com/juju/juju/internal/testhelpers"
)

var renderer = &shell.BashRenderer{}

type confSuite struct {
	testhelpers.IsolationSuite
}

func TestConfSuite(t *testing.T) {
	tc.Run(t, &confSuite{})
}

func (*confSuite) TestIsZeroTrue(c *tc.C) {
	var conf common.Conf
	isZero := conf.IsZero()

	c.Check(isZero, tc.IsTrue)
}

func (*confSuite) TestIsZero(c *tc.C) {
	conf := common.Conf{
		Desc:      "some service",
		ExecStart: "/path/to/some-command a b c",
	}
	isZero := conf.IsZero()

	c.Check(isZero, tc.IsFalse)
}

func (*confSuite) TestValidateOkay(c *tc.C) {
	conf := common.Conf{
		Desc:      "some service",
		ExecStart: "/path/to/some-command a b c",
	}
	err := conf.Validate(renderer)

	c.Check(err, tc.ErrorIsNil)
}

func (*confSuite) TestValidateSingleQuotedExecutable(c *tc.C) {
	conf := common.Conf{
		Desc:      "some service",
		ExecStart: "'/path/to/some-command' a b c",
	}
	err := conf.Validate(renderer)

	c.Check(err, tc.ErrorIsNil)
}

func (*confSuite) TestValidateDoubleQuotedExecutable(c *tc.C) {
	conf := common.Conf{
		Desc:      "some service",
		ExecStart: `"/path/to/some-command" a b c`,
	}
	err := conf.Validate(renderer)

	c.Check(err, tc.ErrorIsNil)
}

func (*confSuite) TestValidatePartiallyQuotedExecutable(c *tc.C) {
	conf := common.Conf{
		Desc:      "some service",
		ExecStart: "'/path/to/some-command a b c'",
	}
	err := conf.Validate(renderer)

	c.Check(err, tc.ErrorMatches, `.*relative path in ExecStart \(.*`)
}

func (*confSuite) TestValidateMissingDesc(c *tc.C) {
	conf := common.Conf{
		ExecStart: "/path/to/some-command a b c",
	}
	err := conf.Validate(renderer)

	c.Check(err, tc.ErrorMatches, ".*missing Desc.*")
}

func (*confSuite) TestValidateMissingExecStart(c *tc.C) {
	conf := common.Conf{
		Desc: "some service",
	}
	err := conf.Validate(renderer)

	c.Check(err, tc.ErrorMatches, ".*missing ExecStart.*")
}

func (*confSuite) TestValidateRelativeExecStart(c *tc.C) {
	conf := common.Conf{
		Desc:      "some service",
		ExecStart: "some-command a b c",
	}
	err := conf.Validate(renderer)

	c.Check(err, tc.ErrorMatches, `.*relative path in ExecStart \(.*`)
}

func (*confSuite) TestValidateRelativeExecStopPost(c *tc.C) {
	conf := common.Conf{
		Desc:         "some service",
		ExecStart:    "/path/to/some-command a b c",
		ExecStopPost: "some-other-command a b c",
	}
	err := conf.Validate(renderer)

	c.Check(err, tc.ErrorMatches, `.*relative path in ExecStopPost \(.*`)
}

func (*confSuite) TestGoodLimits(c *tc.C) {
	conf := common.Conf{
		Desc:      "some service",
		ExecStart: "/path/to/some-command a b c",
		Limit: map[string]string{
			"an-int":    "42",
			"infinity":  "infinity",
			"unlimited": "unlimited",
		},
	}
	err := conf.Validate(renderer)
	c.Check(err, tc.ErrorIsNil)
}

func (*confSuite) TestLimitNotInt(c *tc.C) {
	conf := common.Conf{
		Desc:      "some service",
		ExecStart: "/path/to/some-command a b c",
		Limit: map[string]string{
			"float": "42.5",
		},
	}
	err := conf.Validate(renderer)
	c.Check(err, tc.ErrorMatches, `limit must be "infinity", "unlimited", or an integer, "42.5" not valid`)
}
