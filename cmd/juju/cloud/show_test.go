// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/cloud"
	_ "github.com/juju/juju/provider/all"
	"github.com/juju/juju/testing"
)

type showSuite struct {
	testing.FakeJujuXDGDataHomeSuite
}

var _ = gc.Suite(&showSuite{})

func (s *showSuite) TestShowBadArgs(c *gc.C) {
	_, err := testing.RunCommand(c, cloud.NewShowCloudCommand())
	c.Assert(err, gc.ErrorMatches, "no cloud specified")
}

func (s *showSuite) TestShow(c *gc.C) {
	ctx, err := testing.RunCommand(c, cloud.NewShowCloudCommand(), "aws-china")
	c.Assert(err, jc.ErrorIsNil)
	out := testing.Stdout(ctx)
	c.Assert(out, gc.Equals, `
defined: public
type: ec2
auth-types: [access-key]
regions:
  cn-north-1:
    endpoint: https://ec2.cn-north-1.amazonaws.com.cn/
`[1:])
}
