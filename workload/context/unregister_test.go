// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/workload"
	"github.com/juju/juju/workload/context"
)

type unregisterSuite struct {
	commandSuite

	details workload.Details
}

var _ = gc.Suite(&unregisterSuite{})

func (s *unregisterSuite) SetUpTest(c *gc.C) {
	s.commandSuite.SetUpTest(c)

	cmd, err := context.NewUnregisterCmd(s.Ctx)
	c.Assert(err, jc.ErrorIsNil)
	s.setCommand(c, "payload-unregister", cmd)
}

func (s *unregisterSuite) TestCommandRegistered(c *gc.C) {
	s.checkCommandRegistered(c)
}

func (s *unregisterSuite) TestHelp(c *gc.C) {
	s.checkHelp(c, `
usage: payload-unregister <class> <id>
purpose: stop tracking a payload

"payload-unregister" is used while a hook is running to let Juju know
that a payload has been manually stopped. The <class> and <id> provided
must match a payload that has been previously registered with juju using
payload-register.
`[1:])
}

func (s *unregisterSuite) TestRunOkay(c *gc.C) {
	s.setMetadata(s.workload)
	s.compCtx.workloads[s.workload.ID()] = s.workload
	err := s.cmd.Init([]string{s.workload.Name, s.workload.Details.ID})
	c.Assert(err, jc.ErrorIsNil)

	c.Logf("%#v", s.cmd)

	s.checkRun(c, "", "")
}
