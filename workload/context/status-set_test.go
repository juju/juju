// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/workload"
	"github.com/juju/juju/workload/context"
)

type statusSetSuite struct {
	registeringCommandSuite

	statusSetCmd *context.StatusSetCmd
	details      workload.Details
}

var _ = gc.Suite(&statusSetSuite{})

func (s *statusSetSuite) SetUpTest(c *gc.C) {
	s.commandSuite.SetUpTest(c)

	cmd, err := context.NewStatusSetCmd(s.Ctx)
	c.Assert(err, jc.ErrorIsNil)

	s.statusSetCmd = cmd
	s.setCommand(c, "payload-status-set", s.statusSetCmd)
}

func (s *statusSetSuite) init(c *gc.C, status, id string) {
	err := s.statusSetCmd.Init([]string{status, id})
	c.Assert(err, jc.ErrorIsNil)
	s.details = workload.Details{
		ID: id,
	}
	s.details.Status.State = workload.StateRunning
}

func (s *statusSetSuite) TestHelp(c *gc.C) {
	s.checkHelp(c, `
usage: payload-status-set <status> <id>
purpose: update the status of a payload

"payload-status-set" is used while a hook (update-status) is running to update the
current status of a registered payload. The <id> provided must match a payload that
has been previously registered with juju using payload-register. The <status> must
be on of the follow: starting, started, stopping, stopped
`[1:])
}

func (s *statusSetSuite) TestTooFewArgs(c *gc.C) {
	err := s.statusSetCmd.Init([]string{})
	c.Check(err, gc.ErrorMatches, `missing .*`)

	err = s.statusSetCmd.Init([]string{workload.StateRunning})
	c.Check(err, gc.ErrorMatches, `missing .*`)
}

func (s *statusSetSuite) TestInvalidStatjs(c *gc.C) {
	s.init(c, "created", "foo")
	err := s.cmd.Run(s.cmdCtx)

	c.Check(err, gc.ErrorMatches, `invalid status, .*`)
}

func (s *statusSetSuite) TestStatusSet(c *gc.C) {
	s.init(c, workload.StateStopped, "foo")
	err := s.cmd.Run(s.cmdCtx)

	c.Check(err, jc.ErrorIsNil)
}
