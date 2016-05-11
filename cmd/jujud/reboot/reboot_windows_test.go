// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build windows

package reboot

import (
	"time"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/params"
)

func (s *RebootSuite) TestBuildRebootCommand_ShouldDoNothing(c *gc.C) {
	cmd, args := buildRebootCommand(params.ShouldDoNothing, 0)
	c.Check(cmd, gc.Equals, "")
	c.Check(args, gc.HasLen, 0)
}

func (s *RebootSuite) TestBuildRebootCommand_IsShutdownCmd(c *gc.C) {
	cmd, _ := buildRebootCommand(params.ShouldReboot, 0)
	c.Check(cmd, gc.Equals, "shutdown.exe")
}

func (s *RebootSuite) TestBuildRebootCommand_SetsDelay(c *gc.C) {
	_, args := buildRebootCommand(params.ShouldReboot, 1*time.Minute)
	c.Assert(len(args), gc.Not(jc.LessThan), 2)
	c.Check(args[0], gc.Equals, "-t")
	c.Check(args[1], gc.Equals, "60")
}

func (s *RebootSuite) TestBuildRebootCommand_DelayToCeilingSecond(c *gc.C) {
	_, args := buildRebootCommand(params.ShouldReboot, 2*time.Microsecond)
	c.Assert(len(args), gc.Not(jc.LessThan), 2)
	c.Check(args[0], gc.Equals, "-t")
	c.Check(args[1], gc.Equals, "1")
}

func (s *RebootSuite) TestBuildRebootCommand_RebootSetsRFlag(c *gc.C) {
	_, args := buildRebootCommand(params.ShouldReboot, 0)
	c.Assert(len(args), gc.Not(jc.LessThan), 2)
	c.Check(args[2], gc.Equals, "-r")
}

func (s *RebootSuite) TestBuildRebootCommand_HaltSetsSFlag(c *gc.C) {
	_, args := buildRebootCommand(params.ShouldShutdown, 0)
	c.Assert(len(args), gc.Not(jc.LessThan), 2)
	c.Check(args[2], gc.Equals, "-s")
}
