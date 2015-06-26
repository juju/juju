// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/service"
	// Bring in the dummy provider definition.
	_ "github.com/juju/juju/provider/dummy"
	"github.com/juju/juju/testing"
)

type ServiceCommandSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&ServiceCommandSuite{})

var expectedCommmandNames = []string{
	"add-unit",
	"get",
	"get-constraints",
	"help",
	"set",
	"set-constraints",
	"unset",
}

func (s *ServiceCommandSuite) TestHelp(c *gc.C) {
	// Check the help output
	ctx, err := testing.RunCommand(c, service.NewSuperCommand(), "--help")
	c.Assert(err, jc.ErrorIsNil)
	namesFound := testing.ExtractCommandsFromHelpOutput(ctx)
	c.Assert(namesFound, gc.DeepEquals, expectedCommmandNames)
}
