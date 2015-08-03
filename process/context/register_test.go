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
)

type registerSuite struct {
	registeringCommandSuite

	registerCmd *context.ProcRegistrationCommand
	details     process.Details
}

var _ = gc.Suite(&registerSuite{})

func (s *registerSuite) SetUpTest(c *gc.C) {
	s.commandSuite.SetUpTest(c)

	cmd, err := context.NewProcRegistrationCommand(s.Ctx)
	c.Assert(err, jc.ErrorIsNil)

	s.registerCmd = cmd
	s.setCommand(c, "process-register", s.registerCmd)
}

func (s *registerSuite) init(c *gc.C, name, id, status string) {
	err := s.registerCmd.Init([]string{
		name,
		`{"id":"` + id + `", "status":{"state":"` + status + `"}}`,
	})
	c.Assert(err, jc.ErrorIsNil)
	s.details = process.Details{
		ID:     id,
		Status: process.PluginStatus{State: status},
	}
}

func (s *registerSuite) checkRunInfo(c *gc.C, orig, sent process.Info) {
	s.registeringCommandSuite.checkRunInfo(c, orig, sent)

	c.Check(s.registerCmd.UpdatedProcess, jc.DeepEquals, &sent.Process)
}

func (s *registerSuite) TestCommandRegistered(c *gc.C) {
	s.checkCommandRegistered(c)
}

func (s *registerSuite) TestHelp(c *gc.C) {
	s.checkHelp(c, `
usage: process-register [options] <name-or-id> <proc-details>
purpose: register a workload process

options:
--definition  (= )
    process definition filename (use "-" for STDIN)
--extend  (= )
    extend process definition
--override  (= )
    override process definition

"process-register" is used while a hook is running to let Juju know
that a workload process has been manually started. The information
used to start the process must be provided when "register" is run.

The process name must correspond to one of the processes defined in
the charm's metadata.yaml.
`[1:])
}

func (s *registerSuite) TestInitAllArgs(c *gc.C) {
	err := s.registerCmd.Init([]string{
		s.proc.Name,
		`{"id":"abc123", "status":{"state":"okay"}}`,
	})
	c.Assert(err, jc.ErrorIsNil)

	c.Check(s.registerCmd.Name, gc.Equals, s.proc.Name)
	c.Check(s.registerCmd.Details, jc.DeepEquals, process.Details{
		ID:     "abc123",
		Status: process.PluginStatus{State: "okay"},
	})
}

func (s *registerSuite) TestInitTooFewArgs(c *gc.C) {
	err := s.registerCmd.Init([]string{})
	c.Check(err, gc.ErrorMatches, `missing args .*`)

	err = s.registerCmd.Init([]string{s.proc.Name})
	c.Check(err, gc.ErrorMatches, `missing args .*`)
}

func (s *registerSuite) TestInitTooManyArgs(c *gc.C) {
	err := s.registerCmd.Init([]string{
		s.proc.Name,
		`{"id":"abc123", "status":{"state":"okay"}}`,
		"other",
	})

	c.Check(err, gc.ErrorMatches, "unrecognized args: .*")
}

func (s *registerSuite) TestInitEmptyName(c *gc.C) {
	err := s.registerCmd.Init([]string{
		"",
		"abc123",
	})

	c.Check(err, gc.ErrorMatches, "got empty name-or-id")
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
		`{"status":{"state":"okay"}}`,
	})

	c.Check(err, jc.Satisfies, errors.IsNotValid)
}

func (s *registerSuite) TestInitMissingDetailsStatus(c *gc.C) {
	err := s.registerCmd.Init([]string{
		s.proc.Name,
		`{"id":"abc123"}`,
	})

	c.Check(err, jc.Satisfies, errors.IsNotValid)
}

func (s *registerSuite) TestInitBadJSON(c *gc.C) {
	err := s.registerCmd.Init([]string{
		s.proc.Name,
		`{"id":"abc123", "status":{"state":"okay"}`,
	})

	c.Check(errors.Cause(err), gc.ErrorMatches, "unexpected end of JSON input")
}

func (s *registerSuite) TestOverridesWithoutSubfield(c *gc.C) {
	s.proc.Process.Description = "notFoo"
	s.setMetadata(*s.proc)
	s.registerCmd.Overrides = []string{
		"description:foo",
	}
	s.init(c, s.proc.Name, "abc123-override", "okay")

	def := s.proc.Process.Copy()
	def.Description = "foo"
	s.checkRunInfo(c, *s.proc, process.Info{
		Process: def,
		Status:  process.Status{State: process.StateRunning},
		Details: s.details,
	})
}

func (s *registerSuite) TestOverridesWithSubfield(c *gc.C) {
	s.proc.Process.EnvVars = map[string]string{"foo": "bar"}
	s.setMetadata(*s.proc)
	s.registerCmd.Overrides = []string{
		"env/foo:baz",
	}
	s.init(c, s.proc.Name, "abc123-override", "okay")

	def := s.proc.Process.Copy()
	def.EnvVars = map[string]string{"foo": "baz"}
	s.checkRunInfo(c, *s.proc, process.Info{
		Process: def,
		Status:  process.Status{State: process.StateRunning},
		Details: s.details,
	})
}

func (s *registerSuite) TestOverridesMissingField(c *gc.C) {
	s.setMetadata(*s.proc)
	s.registerCmd.Overrides = []string{
		":value",
	}
	s.init(c, s.proc.Name, "abc123-override", "okay")

	err := s.cmd.Run(s.cmdCtx)

	c.Check(err, gc.ErrorMatches, "override: missing field")
}

func (s *registerSuite) TestOverridesMissingValue(c *gc.C) {
	s.setMetadata(*s.proc)
	s.registerCmd.Overrides = []string{
		"field:",
	}
	s.init(c, s.proc.Name, "abc123-override", "okay")

	err := s.cmd.Run(s.cmdCtx)

	c.Check(err, gc.ErrorMatches, "override: missing value")
}

func (s *registerSuite) TestOverridesMissingColon(c *gc.C) {
	s.setMetadata(*s.proc)
	s.registerCmd.Overrides = []string{
		"fieldvalue",
	}
	s.init(c, s.proc.Name, "abc123-override", "okay")

	err := s.cmd.Run(s.cmdCtx)

	c.Check(err, gc.ErrorMatches, "override: missing value")
}

func (s *registerSuite) TestAdditionsWithoutSubfield(c *gc.C) {
	s.proc.Process.Description = ""
	s.setMetadata(*s.proc)
	s.registerCmd.Additions = []string{
		"description:foo",
	}
	s.init(c, s.proc.Name, "abc123-override", "okay")

	def := s.proc.Process.Copy()
	def.Description = "foo"
	s.checkRunInfo(c, *s.proc, process.Info{
		Process: def,
		Status:  process.Status{State: process.StateRunning},
		Details: s.details,
	})
}

func (s *registerSuite) TestAdditionsWithSubfield(c *gc.C) {
	s.setMetadata(*s.proc)
	s.registerCmd.Additions = []string{
		"env/foo:baz",
	}
	s.init(c, s.proc.Name, "abc123-override", "okay")

	def := s.proc.Process.Copy()
	def.EnvVars = map[string]string{"foo": "baz"}
	s.checkRunInfo(c, *s.proc, process.Info{
		Process: def,
		Status:  process.Status{State: process.StateRunning},
		Details: s.details,
	})
}

func (s *registerSuite) TestAdditionsMissingField(c *gc.C) {
	s.setMetadata(*s.proc)
	s.registerCmd.Additions = []string{
		":value",
	}
	s.init(c, s.proc.Name, "abc123-override", "okay")

	err := s.cmd.Run(s.cmdCtx)

	c.Check(err, gc.ErrorMatches, "extend: missing field")
}

func (s *registerSuite) TestAdditionsMissingValue(c *gc.C) {
	s.setMetadata(*s.proc)
	s.registerCmd.Additions = []string{
		"field:",
	}
	s.init(c, s.proc.Name, "abc123-override", "okay")

	err := s.cmd.Run(s.cmdCtx)

	c.Check(err, gc.ErrorMatches, "extend: missing value")
}

func (s *registerSuite) TestAdditionMissingColon(c *gc.C) {
	s.setMetadata(*s.proc)
	s.registerCmd.Additions = []string{
		"fieldvalue",
	}
	s.init(c, s.proc.Name, "abc123-override", "okay")

	err := s.cmd.Run(s.cmdCtx)

	c.Check(err, gc.ErrorMatches, "extend: missing value")
}

func (s *registerSuite) TestRunOkay(c *gc.C) {
	s.setMetadata(*s.proc)
	s.init(c, s.proc.Name, "abc123", "running")

	s.checkRun(c, "", "")
	s.Stub.CheckCallNames(c, "List", "ListDefinitions", "Set", "Flush")
}

func (s *registerSuite) TestRunAlreadyRegistered(c *gc.C) {
	s.proc.Details.ID = "xyz123"
	s.compCtx.procs[s.proc.Name] = s.proc
	s.init(c, s.proc.Name, "abc123", "okay")

	err := s.registerCmd.Run(s.cmdCtx)

	c.Check(err, gc.ErrorMatches, ".*already registered")
}

func (s *registerSuite) TestRunUpdatedProcess(c *gc.C) {
	s.proc.Process.Description = "bar"
	s.setMetadata(*s.proc)
	s.registerCmd.Overrides = []string{"description:foo"}
	s.init(c, s.proc.Name, "abc123", "running")

	s.checkRun(c, "", "")

	s.proc.Process = *s.registerCmd.UpdatedProcess
	s.proc.Status.State = process.StateRunning
	s.proc.Details = s.details
	s.Stub.CheckCalls(c, []testing.StubCall{{
		FuncName: "List",
	}, {
		FuncName: "ListDefinitions",
	}, {
		FuncName: "Set",
		Args:     []interface{}{*s.proc},
	}, {
		FuncName: "Flush",
		Args:     nil,
	}})
}
