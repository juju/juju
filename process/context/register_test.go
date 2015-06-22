// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context_test

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/process"
	"github.com/juju/juju/process/context"
	"github.com/juju/juju/process/plugin"
)

type registerSuite struct {
	commandSuite

	registerCmd *context.ProcRegistrationCommand
	details     plugin.Details
}

var _ = gc.Suite(&registerSuite{})

func (s *registerSuite) SetUpTest(c *gc.C) {
	s.commandSuite.SetUpTest(c)

	cmd, err := context.NewProcRegistrationCommand(s.Ctx)
	c.Assert(err, jc.ErrorIsNil)

	s.registerCmd = cmd
	s.setCommand(c, "register", s.registerCmd)
}

func (s *registerSuite) init(c *gc.C, name, id, status string) {
	err := s.registerCmd.Init([]string{
		name,
		`{"id":"` + id + `", "status":{"status":"` + status + `"}}`,
	})
	c.Assert(err, jc.ErrorIsNil)
	s.details = plugin.Details{
		ID:            id,
		CurrentStatus: plugin.Status{Status: status},
	}
}

func (s *registerSuite) checkRun(c *gc.C, expectedOut, expectedErr string) {
	s.commandSuite.checkRun(c, expectedOut, expectedErr)

	s.checkStatus(c, process.StatusActive)
	s.checkDetails(c, s.details)
}

func (s *registerSuite) TestCommandRegistered(c *gc.C) {
	s.checkCommandRegistered(c)
}

func (s *registerSuite) TestHelp(c *gc.C) {
	s.checkHelp(c, `
usage: register [options] <name> <proc-details>
purpose: register a workload process

options:
--definition  (= )
    process definition filename (use "-" for STDIN)
--extend  (= )
    extend process definition
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
		`{"id":"abc123", "status":{"status":"okay"}}`,
	})
	c.Assert(err, jc.ErrorIsNil)

	c.Check(s.registerCmd.Name, gc.Equals, s.proc.Name)
	c.Check(s.registerCmd.Details, jc.DeepEquals, plugin.Details{
		ID:            "abc123",
		CurrentStatus: plugin.Status{Status: "okay"},
	})
}

func (s *registerSuite) TestInitTooFewArgs(c *gc.C) {
	err := s.registerCmd.Init([]string{})
	c.Check(err, gc.ErrorMatches, "expected <name> <proc-details>, got: .*")

	err = s.registerCmd.Init([]string{s.proc.Name})
	c.Check(err, gc.ErrorMatches, "expected <name> <proc-details>, got: .*")
}

func (s *registerSuite) TestInitTooManyArgs(c *gc.C) {
	err := s.registerCmd.Init([]string{
		s.proc.Name,
		`{"id":"abc123", "status":{"status":"okay"}}`,
		"other",
	})

	c.Check(err, gc.ErrorMatches, "expected <name> <proc-details>, got: .*")
}

func (s *registerSuite) TestInitEmptyName(c *gc.C) {
	err := s.registerCmd.Init([]string{
		"",
		"abc123",
	})

	c.Check(err, gc.ErrorMatches, "got empty name")
}

func (s *registerSuite) TestInitEmptyID(c *gc.C) {
	err := s.registerCmd.Init([]string{
		s.proc.Name,
		"",
	})

	c.Check(errors.Cause(err), gc.ErrorMatches, "unexpected end of JSON input")
}

func (s *registerSuite) TestInitMissingDetailsID(c *gc.C) {
	err := s.registerCmd.Init([]string{
		s.proc.Name,
		`{"status":{"status":"okay"}}`,
	})

	c.Check(errors.Cause(err), jc.Satisfies, plugin.IsInvalid)
}

func (s *registerSuite) TestInitMissingDetailsStatus(c *gc.C) {
	err := s.registerCmd.Init([]string{
		s.proc.Name,
		`{"id":"abc123"}`,
	})

	c.Check(errors.Cause(err), jc.Satisfies, plugin.IsInvalid)
}

func (s *registerSuite) TestInitBadJSON(c *gc.C) {
	err := s.registerCmd.Init([]string{
		s.proc.Name,
		`{"id":"abc123", "status":"okay"`,
	})

	c.Check(errors.Cause(err), gc.ErrorMatches, "unexpected end of JSON input")
}

func (s *registerSuite) TestOverridesWithoutSubfield(c *gc.C) {
	s.proc.Process.Description = "notFoo"
	s.registerCmd.Overrides = []string{
		"description:foo",
	}
	s.init(c, s.proc.Name, "abc123-override", "okay")

	s.checkRun(c, "", "")

	expected := s.proc.Process.Copy()
	expected.Description = "foo"
	c.Check(s.registerCmd.UpdatedProcess, jc.DeepEquals, &expected)
}

func (s *registerSuite) TestOverridesWithSubfield(c *gc.C) {
	s.proc.Process.EnvVars = map[string]string{"foo": "bar"}
	s.registerCmd.Overrides = []string{
		"env/foo:baz",
	}
	s.init(c, s.proc.Name, "abc123-override", "okay")

	s.checkRun(c, "", "")

	expected := s.proc.Process.Copy()
	expected.EnvVars = map[string]string{"foo": "baz"}
	c.Check(s.registerCmd.UpdatedProcess, jc.DeepEquals, &expected)
}

func (s *registerSuite) TestOverridesMissingField(c *gc.C) {
	s.registerCmd.Overrides = []string{
		":value",
	}
	s.init(c, s.proc.Name, "abc123-override", "okay")

	err := s.cmd.Run(s.cmdCtx)

	c.Check(err, gc.ErrorMatches, "override: missing field")
}

func (s *registerSuite) TestOverridesMissingValue(c *gc.C) {
	s.registerCmd.Overrides = []string{
		"field:",
	}
	s.init(c, s.proc.Name, "abc123-override", "okay")

	err := s.cmd.Run(s.cmdCtx)

	c.Check(err, gc.ErrorMatches, "override: missing value")
}

func (s *registerSuite) TestOverridesMissingColon(c *gc.C) {
	s.registerCmd.Overrides = []string{
		"fieldvalue",
	}
	s.init(c, s.proc.Name, "abc123-override", "okay")

	err := s.cmd.Run(s.cmdCtx)

	c.Check(err, gc.ErrorMatches, "override: missing value")
}

func (s *registerSuite) TestAdditionsWithoutSubfield(c *gc.C) {
	s.proc.Process.Description = ""
	s.registerCmd.Additions = []string{
		"description:foo",
	}
	s.init(c, s.proc.Name, "abc123-override", "okay")

	s.checkRun(c, "", "")

	expected := s.proc.Process.Copy()
	expected.Description = "foo"
	c.Check(s.registerCmd.UpdatedProcess, jc.DeepEquals, &expected)
}

func (s *registerSuite) TestAdditionsWithSubfield(c *gc.C) {
	s.registerCmd.Additions = []string{
		"env/foo:baz",
	}
	s.init(c, s.proc.Name, "abc123-override", "okay")

	s.checkRun(c, "", "")

	expected := s.proc.Process.Copy()
	expected.EnvVars = map[string]string{"foo": "baz"}
	c.Check(s.registerCmd.UpdatedProcess, jc.DeepEquals, &expected)
}

func (s *registerSuite) TestAdditionsMissingField(c *gc.C) {
	s.registerCmd.Additions = []string{
		":value",
	}
	s.init(c, s.proc.Name, "abc123-override", "okay")

	err := s.cmd.Run(s.cmdCtx)

	c.Check(err, gc.ErrorMatches, "extend: missing field")
}

func (s *registerSuite) TestAdditionsMissingValue(c *gc.C) {
	s.registerCmd.Additions = []string{
		"field:",
	}
	s.init(c, s.proc.Name, "abc123-override", "okay")

	err := s.cmd.Run(s.cmdCtx)

	c.Check(err, gc.ErrorMatches, "extend: missing value")
}

func (s *registerSuite) TestAdditionMissingColon(c *gc.C) {
	s.registerCmd.Additions = []string{
		"fieldvalue",
	}
	s.init(c, s.proc.Name, "abc123-override", "okay")

	err := s.cmd.Run(s.cmdCtx)

	c.Check(err, gc.ErrorMatches, "extend: missing value")
}

func (s *registerSuite) TestRunOkay(c *gc.C) {
	s.init(c, s.proc.Name, "abc123", "running")

	s.checkRun(c, "", "")
	s.Stub.CheckCallNames(c, "Set")
}

func (s *registerSuite) TestRunUpdatedProcess(c *gc.C) {
	s.proc.Process.Description = "bar"
	s.registerCmd.Overrides = []string{"description:foo"}
	s.init(c, s.proc.Name, "abc123", "running")

	s.checkRun(c, "", "")

	s.proc.Process = *s.registerCmd.UpdatedProcess
	s.proc.Status = process.StatusActive
	s.proc.Details = s.details
	s.Stub.CheckCalls(c, []testing.StubCall{{
		FuncName: "Set",
		Args:     []interface{}{s.proc.Name, s.proc},
	}})
}

func (s *registerSuite) TestRunAlreadyRegistered(c *gc.C) {
	s.init(c, s.proc.Name, "abc123", "running")
	context.GetCmdInfo(s.cmd).Status = process.StatusActive

	err := s.cmd.Run(s.cmdCtx)

	c.Check(err, gc.ErrorMatches, "already registered")
}
