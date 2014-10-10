// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package action_test

import (
	"github.com/juju/juju/cmd/juju/action"
	gc "gopkg.in/check.v1"
)

type DoSuite struct {
	BaseActionSuite
	UndefinedActionCommandSuite
	subcommand *action.DoCommand
}

func (s *DoSuite) SetUpTest(c *gc.C) {
	s.BaseActionSuite.SetUpTest(c)
	s.subcommand = &action.DoCommand{}
}

func (s *DoSuite) TestHelp(c *gc.C) {
	s.checkHelp(c, s.subcommand)
}
