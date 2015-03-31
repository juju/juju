// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/shell"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/service/common"
)

var renderer = &shell.BashRenderer{}

type confSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&confSuite{})

func (*confSuite) TestIsZeroTrue(c *gc.C) {
	var conf common.Conf
	isZero := conf.IsZero()

	c.Check(isZero, jc.IsTrue)
}

func (*confSuite) TestIsZero(c *gc.C) {
	conf := common.Conf{
		Desc:      "some service",
		ExecStart: "/path/to/some-command a b c",
	}
	isZero := conf.IsZero()

	c.Check(isZero, jc.IsFalse)
}

func (*confSuite) TestValidateOkay(c *gc.C) {
	conf := common.Conf{
		Desc:      "some service",
		ExecStart: "/path/to/some-command a b c",
	}
	err := conf.Validate(renderer)

	c.Check(err, jc.ErrorIsNil)
}

func (*confSuite) TestValidateSingleQuotedExecutable(c *gc.C) {
	conf := common.Conf{
		Desc:      "some service",
		ExecStart: "'/path/to/some-command' a b c",
	}
	err := conf.Validate(renderer)

	c.Check(err, jc.ErrorIsNil)
}

func (*confSuite) TestValidateDoubleQuotedExecutable(c *gc.C) {
	conf := common.Conf{
		Desc:      "some service",
		ExecStart: `"/path/to/some-command" a b c`,
	}
	err := conf.Validate(renderer)

	c.Check(err, jc.ErrorIsNil)
}

func (*confSuite) TestValidatePartiallyQuotedExecutable(c *gc.C) {
	conf := common.Conf{
		Desc:      "some service",
		ExecStart: "'/path/to/some-command a b c'",
	}
	err := conf.Validate(renderer)

	c.Check(err, gc.ErrorMatches, `.*relative path in ExecStart \(.*`)
}

func (*confSuite) TestValidateMissingDesc(c *gc.C) {
	conf := common.Conf{
		ExecStart: "/path/to/some-command a b c",
	}
	err := conf.Validate(renderer)

	c.Check(err, gc.ErrorMatches, ".*missing Desc.*")
}

func (*confSuite) TestValidateMissingExecStart(c *gc.C) {
	conf := common.Conf{
		Desc: "some service",
	}
	err := conf.Validate(renderer)

	c.Check(err, gc.ErrorMatches, ".*missing ExecStart.*")
}

func (*confSuite) TestValidateRelativeExecStart(c *gc.C) {
	conf := common.Conf{
		Desc:      "some service",
		ExecStart: "some-command a b c",
	}
	err := conf.Validate(renderer)

	c.Check(err, gc.ErrorMatches, `.*relative path in ExecStart \(.*`)
}

func (*confSuite) TestValidateRelativeExecStopPost(c *gc.C) {
	conf := common.Conf{
		Desc:         "some service",
		ExecStart:    "/path/to/some-command a b c",
		ExecStopPost: "some-other-command a b c",
	}
	err := conf.Validate(renderer)

	c.Check(err, gc.ErrorMatches, `.*relative path in ExecStopPost \(.*`)
}
