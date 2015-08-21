// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/process"
	"github.com/juju/juju/process/context"
)

type untrackSuite struct {
	commandSuite

	cmd     *context.UntrackCmd
	details process.Details
}

var _ = gc.Suite(&untrackSuite{})

func (s *untrackSuite) SetUpTest(c *gc.C) {
	s.commandSuite.SetUpTest(c)

	var err error
	s.cmd, err = context.NewUntrackCmd(s.Ctx)
	c.Assert(err, jc.ErrorIsNil)
	s.setCommand(c, "workload-untrack", s.cmd)
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

func (s *untrackSuite) TestInitAllArgs(c *gc.C) {
	err := s.cmd.Init([]string{s.proc.Name})
	c.Assert(err, jc.ErrorIsNil)

	c.Check(context.Name(s.cmd), gc.Equals, s.proc.Name)
}

func (s *untrackSuite) TestInitTooFewArgs(c *gc.C) {
	err := s.cmd.Init([]string{})
	c.Check(err, gc.ErrorMatches, `missing arg .*`)
}

func (s *untrackSuite) TestInitTooManyArgs(c *gc.C) {
	err := s.cmd.Init([]string{
		s.proc.Name,
		`{"id":"abc123", "status":{"state":"okay"}}`,
		"other",
	})

	c.Check(err, gc.ErrorMatches, "unexpected args: .*")
}

func (s *untrackSuite) TestInitEmptyName(c *gc.C) {
	err := s.cmd.Init([]string{""})

	c.Check(err, gc.ErrorMatches, context.ArgNameOrId+" cannot be empty")
}

func (s *untrackSuite) TestRunOkay(c *gc.C) {
	s.setMetadata(s.proc)
	s.compCtx.procs[s.proc.ID()] = s.proc
	s.cmd.Init([]string{s.proc.Name})

	s.checkRun(c, "", "")
}
