// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context_test

import (
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/process"
	"github.com/juju/juju/process/context"
)

type registerSuite struct {
	commandSuite

	registerCmd *context.RegisterCommand
}

var _ = gc.Suite(&registerSuite{})

func (s *registerSuite) SetUpTest(c *gc.C) {
	s.commandSuite.SetUpTest(c)

	s.registerCmd = context.NewRegisterCommand(s.Ctx)
	s.setCommand(c, "register", s.registerCmd)
}

func (s *registerSuite) init(c *gc.C, name, id, status string) {
	s.registerCmd.Init([]string{s.proc.Name, "abc123"})
}

func (s *registerSuite) TestCommandRegistered(c *gc.C) {
	s.checkCommandRegistered(c)
}

func (s *registerSuite) TestHelp(c *gc.C) {
	s.checkHelp(c, `
usage: register [options] <name> <id> [<details>]
purpose: register a workload process

options:
--override  (= )
    override process definition

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

func (s *registerSuite) TestInitOverridesWithoutSubfield(c *gc.C) {
	s.proc.Process.Description = "notFoo"
	s.registerCmd.Overrides = []string{
		"description:foo",
	}

	err := s.registerCmd.Init([]string{
		s.proc.Name,
		"abc123-override",
	})
	c.Assert(err, jc.ErrorIsNil)

	expected := s.proc.Process.Copy()
	expected.Description = "foo"
	c.Check(s.registerCmd.UpdatedProcess, jc.DeepEquals, &expected)
}

func (s *registerSuite) TestInitOverridesWithSubfield(c *gc.C) {
	s.proc.Process.EnvVars = map[string]string{"foo": "bar"}
	s.registerCmd.Overrides = []string{
		"env/foo:baz",
	}

	err := s.registerCmd.Init([]string{
		s.proc.Name,
		"abc123-override",
	})
	c.Assert(err, jc.ErrorIsNil)

	expected := s.proc.Process.Copy()
	expected.EnvVars = map[string]string{"foo": "baz"}
	c.Check(s.registerCmd.UpdatedProcess, jc.DeepEquals, &expected)
}

func (s *registerSuite) TestInitOverridesMissingField(c *gc.C) {
	s.registerCmd.Overrides = []string{
		":value",
	}

	err := s.registerCmd.Init([]string{
		s.proc.Name,
		"abc123-override",
	})
	c.Assert(err, gc.ErrorMatches, "missing override field")
}

func (s *registerSuite) TestInitOverridesMissingValue(c *gc.C) {
	s.registerCmd.Overrides = []string{
		"field:",
	}

	err := s.registerCmd.Init([]string{
		s.proc.Name,
		"abc123-override",
	})
	c.Assert(err, gc.ErrorMatches, "missing override value")
}

func (s *registerSuite) TestInitOverridesMissingColon(c *gc.C) {
	s.registerCmd.Overrides = []string{
		"fieldvalue",
	}

	err := s.registerCmd.Init([]string{
		s.proc.Name,
		"abc123-override",
	})
	c.Assert(err, gc.ErrorMatches, "missing override value")
}

func (s *registerSuite) TestRunOkay(c *gc.C) {
	s.init(c, s.proc.Name, "abc123", "running")

	s.checkRun(c, "", "")
	s.Stub.CheckCallNames(c, "Set")
}

func (s *registerSuite) TestRunUpdatedProcess(c *gc.C) {
	s.init(c, s.proc.Name, "abc123", "running")

	s.proc.Process.Description = "bar"
	s.registerCmd.Overrides = []string{"description:foo"}

	s.checkRun(c, "", "")

	s.proc.Process = *s.registerCmd.UpdatedProcess
	s.Stub.CheckCalls(c, []testing.StubCall{{
		FuncName: "Set",
		Args:     []interface{}{s.proc.Name, s.proc},
	}})
}

func (s *registerSuite) TestRunAlreadyRegistered(c *gc.C) {
	s.proc.Status = process.StatusActive
	s.init(c, s.proc.Name, "abc123", "running")

	err := s.cmd.Run(s.cmdCtx)

	c.Check(err, gc.ErrorMatches, "already registered")
}
