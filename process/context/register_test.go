// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/process"
	"github.com/juju/juju/process/context"
	"github.com/juju/juju/worker/uniter/runner/jujuc"
)

type registerSuite struct {
	commandSuite

	registerCmd *context.RegisterCommand
}

var _ = gc.Suite(&registerSuite{})

func (s *registerSuite) SetUpTest(c *gc.C) {
	s.commandSuite.SetUpTest(c)

	registerCmd := context.NewRegisterCommand(s.Ctx.Context)
	s.cmdName = "register" + jujuc.CmdSuffix
	s.cmd = registerCmd
	s.registerCmd = registerCmd
}

func (s *registerSuite) init(c *gc.C, name, id, status string) {
	s.registerCmd.Init([]string{s.proc.Name, "abc123"})
	//s.registerCmd.Details = ...
	//s.registerCmd.Space = ...
	//s.registerCmd.Env = ...
}

func (s *registerSuite) TestCommandRegistered(c *gc.C) {
	s.checkCommandRegistered(c)
}

func (s *registerSuite) TestHelp(c *gc.C) {
	s.checkHelp(c, `
usage: register [options] <name> <id> [<details>]
purpose: register a workload process

options:
--env  (= )
    environment variables
--space (= "")
    network space

"register" is used while a hook is running to let Juju know that
a workload process has been manually started. The information used
to start the process must be provided when "register" is run.

The process name must correspond to one of the processes defined in
the charm's metadata.yaml.
`[1:])
}

func (s *registerSuite) TestInitAllArgs(c *gc.C) {
	err := s.registerCmd.Init([]string{
		s.proc.Name,
		"abc123",
		`{"id":"abc123", "status":"okay"}`,
	})
	c.Assert(err, jc.ErrorIsNil)

	c.Check(s.registerCmd.Name, gc.Equals, s.proc.Name)
	c.Check(s.registerCmd.Id, gc.Equals, "abc123")
	c.Check(s.registerCmd.Details, jc.DeepEquals, process.LaunchDetails{
		UniqueID: "abc123",
		Status:   "okay",
	})
	c.Check(s.registerCmd.Space, gc.Equals, "")
	c.Check(s.registerCmd.Env, gc.HasLen, 0)
}

func (s *registerSuite) TestInitMinArgs(c *gc.C) {
	err := s.registerCmd.Init([]string{
		s.proc.Name,
		"abc123",
	})
	c.Assert(err, jc.ErrorIsNil)

	c.Check(s.registerCmd.Name, gc.Equals, s.proc.Name)
	c.Check(s.registerCmd.Id, gc.Equals, "abc123")
	c.Check(s.registerCmd.Details, jc.DeepEquals, process.LaunchDetails{})
	c.Check(s.registerCmd.Space, gc.Equals, "")
	c.Check(s.registerCmd.Env, gc.HasLen, 0)
}

func (s *registerSuite) TestInitTooFewArgs(c *gc.C) {
	err := s.registerCmd.Init([]string{})
	c.Check(err, gc.ErrorMatches, "expected at least 2 args, got: .*")

	err = s.registerCmd.Init([]string{s.proc.Name})
	c.Check(err, gc.ErrorMatches, "expected at least 2 args, got: .*")
}

func (s *registerSuite) TestInitTooManyArgs(c *gc.C) {
	err := s.registerCmd.Init([]string{
		s.proc.Name,
		"abc123",
		`{"id":"abc123", "status":"okay"}`,
		"other",
	})

	c.Check(err, gc.ErrorMatches, "expected at most 3 args, got: .*")
}

func (s *registerSuite) TestInitEmptyName(c *gc.C) {
	err := s.registerCmd.Init([]string{
		"",
		"abc123",
	})

	c.Check(err, gc.ErrorMatches, "got empty name")
}

func (s *registerSuite) TestInitEmptyId(c *gc.C) {
	err := s.registerCmd.Init([]string{
		s.proc.Name,
		"",
	})

	c.Check(err, gc.ErrorMatches, "got empty id")
}

func (s *registerSuite) TestInitMissingDetailsId(c *gc.C) {
	err := s.registerCmd.Init([]string{
		s.proc.Name,
		"abc123",
		`{"status":"okay"}`,
	})

	c.Check(err, gc.ErrorMatches, "UniqueID must be set")
}

func (s *registerSuite) TestInitMissingDetailsStatus(c *gc.C) {
	err := s.registerCmd.Init([]string{
		s.proc.Name,
		"abc123",
		`{"id":"abc123"}`,
	})

	c.Check(err, gc.ErrorMatches, "Status must be set")
}

func (s *registerSuite) TestInitBadJSON(c *gc.C) {
	err := s.registerCmd.Init([]string{
		s.proc.Name,
		"abc123",
		`{"id":"abc123", "status":"okay"`,
	})

	c.Check(err, gc.ErrorMatches, "unexpected end of JSON input")
}

func (s *registerSuite) TestRun(c *gc.C) {
	s.init(c, s.proc.Name, "abc123", "running")

	s.checkRun(c, "", "")
	// TODO(ericsnow) Check the underlying calls here (s.Stub.CheckCallNames).
}
