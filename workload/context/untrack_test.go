// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/workload"
	"github.com/juju/juju/workload/context"
)

type untrackSuite struct {
	commandSuite

	details workload.Details
}

var _ = gc.Suite(&untrackSuite{})

func (s *untrackSuite) SetUpTest(c *gc.C) {
	s.commandSuite.SetUpTest(c)

	cmd, err := context.NewUntrackCmd(s.Ctx)
	c.Assert(err, jc.ErrorIsNil)
	s.setCommand(c, "workload-untrack", cmd)
}

func (s *untrackSuite) TestCommandRegistered(c *gc.C) {
	s.checkCommandRegistered(c)
}

func (s *untrackSuite) TestHelp(c *gc.C) {
	s.checkHelp(c, `
usage: workload-untrack <name-or-id>
purpose: stop tracking a workload

"workload-untrack" is used while a hook is running to let Juju know
that a workload has been manually stopped. The id
used to start tracking the workload must be provided.
`[1:])
}

func (s *untrackSuite) TestRunOkay(c *gc.C) {
	s.setMetadata(s.workload)
	s.compCtx.workloads[s.workload.ID()] = s.workload
	err := s.cmd.Init([]string{s.workload.Name})
	c.Assert(err, jc.ErrorIsNil)

	c.Logf("%#v", s.cmd)

	s.checkRun(c, "", "")
}
