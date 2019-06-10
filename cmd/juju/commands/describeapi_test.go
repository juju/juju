// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.
package commands

import (
	"github.com/juju/cmd/cmdtesting"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

type DescribeAPISuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&DescribeAPISuite{})

func (s *DescribeAPISuite) TestResult(c *gc.C) {

	ctx, err := cmdtesting.RunCommand(c, newDescribeAPICommon())
	c.Check(err, jc.ErrorIsNil)
	c.Check(cmdtesting.Stdout(ctx), gc.Equals, allFacadesSchema)
}
