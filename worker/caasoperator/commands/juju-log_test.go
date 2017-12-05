// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands_test

import (
	"github.com/juju/cmd"
	"github.com/juju/cmd/cmdtesting"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/worker/caasoperator/commands"
)

type JujuLogSuite struct {
	ContextSuite
}

var _ = gc.Suite(&JujuLogSuite{})

func (s *JujuLogSuite) newJujuLogCommand(c *gc.C) cmd.Command {
	hctx := s.newHookContext(c)
	com, err := commands.NewCommand(hctx, "juju-log")
	c.Assert(err, jc.ErrorIsNil)
	return com
}

func (s *JujuLogSuite) TestRequiresMessage(c *gc.C) {
	com := s.newJujuLogCommand(c)
	cmdtesting.TestInit(c, com, nil, "no message specified")
}

func (s *JujuLogSuite) TestLogInitMissingLevel(c *gc.C) {
	com := s.newJujuLogCommand(c)
	cmdtesting.TestInit(c, com, []string{"-l"}, "flag needs an argument.*")

	com = s.newJujuLogCommand(c)
	cmdtesting.TestInit(c, com, []string{"--log-level"}, "flag needs an argument.*")
}

func (s *JujuLogSuite) TestLogInitMissingMessage(c *gc.C) {
	com := s.newJujuLogCommand(c)
	cmdtesting.TestInit(c, com, []string{"-l", "FATAL"}, "no message specified")

	com = s.newJujuLogCommand(c)
	cmdtesting.TestInit(c, com, []string{"--log-level", "FATAL"}, "no message specified")
}
