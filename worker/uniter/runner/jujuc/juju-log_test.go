// Copyright 2012, 2013 Canonical Ltd.
// Copyright 2014 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc_test

import (
	"github.com/juju/cmd"
	"github.com/juju/cmd/cmdtesting"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/worker/uniter/runner/jujuc"
)

type JujuLogSuite struct {
	relationSuite
}

var _ = gc.Suite(&JujuLogSuite{})

func (s *JujuLogSuite) newJujuLogCommand(c *gc.C) cmd.Command {
	ctx, _ := s.newHookContext(-1, "")
	com, err := jujuc.NewCommand(ctx, cmdString("juju-log"))
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

func (s *JujuLogSuite) TestLogDeprecation(c *gc.C) {
	com := s.newJujuLogCommand(c)
	ctx, err := cmdtesting.RunCommand(c, com, "--format", "foo", "msg")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(ctx), gc.Equals, "--format flag deprecated for command \"juju-log\"")
}
