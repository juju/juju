// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context_test

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/workload"
	"github.com/juju/juju/workload/context"
)

type trackSuite struct {
	registeringCommandSuite

	trackCmd *context.WorkloadTrackCommand
	details  workload.Details
}

var _ = gc.Suite(&trackSuite{})

func (s *trackSuite) SetUpTest(c *gc.C) {
	s.commandSuite.SetUpTest(c)

	cmd, err := context.NewWorkloadTrackCommand(s.Ctx)
	c.Assert(err, jc.ErrorIsNil)

	s.trackCmd = cmd
	s.trackCmd.ReadDefinitions = s.readDefinitions
	s.setCommand(c, "workload-track", s.trackCmd)
}

func (s *trackSuite) init(c *gc.C, name, id, status string) {
	err := s.trackCmd.Init([]string{
		name,
		`{"id":"` + id + `", "status":{"state":"` + status + `"}}`,
	})
	c.Assert(err, jc.ErrorIsNil)
	s.details = workload.Details{
		ID:     id,
		Status: workload.PluginStatus{State: status},
	}
}

func (s *trackSuite) checkRunInfo(c *gc.C, orig, sent workload.Info) {
	s.registeringCommandSuite.checkRunInfo(c, orig, sent)

	c.Check(s.trackCmd.UpdatedWorkload, jc.DeepEquals, &sent.Workload)
}

func (s *trackSuite) TestCommandRegistered(c *gc.C) {
	s.checkCommandRegistered(c)
}

func (s *trackSuite) TestHelp(c *gc.C) {
	s.checkHelp(c, `
usage: workload-track [options] <name-or-id> <workload-details>
purpose: track a workload

options:
--definition  (= )
    workload definition filename (use "-" for STDIN)
--extend  (= )
    extend workload definition
--override  (= )
    override workload definition

"workload-track" is used while a hook is running to let Juju know
that a workload has been started. The information
used to start the workload must be provided when "track" is run.

The workload name must correspond to one of the workloads defined in
the charm's workloads.yaml.
`[1:])
}

func (s *trackSuite) TestInitAllArgs(c *gc.C) {
	err := s.trackCmd.Init([]string{
		s.workload.Name,
		`{"id":"abc123", "status":{"state":"okay"}}`,
	})
	c.Assert(err, jc.ErrorIsNil)

	c.Check(s.trackCmd.Name, gc.Equals, s.workload.Name)
	c.Check(s.trackCmd.Details, jc.DeepEquals, workload.Details{
		ID:     "abc123",
		Status: workload.PluginStatus{State: "okay"},
	})
}

func (s *trackSuite) TestInitTooFewArgs(c *gc.C) {
	err := s.trackCmd.Init([]string{})
	c.Check(err, gc.ErrorMatches, `missing args .*`)

	err = s.trackCmd.Init([]string{s.workload.Name})
	c.Check(err, gc.ErrorMatches, `missing args .*`)
}

func (s *trackSuite) TestInitTooManyArgs(c *gc.C) {
	err := s.trackCmd.Init([]string{
		s.workload.Name,
		`{"id":"abc123", "status":{"state":"okay"}}`,
		"other",
	})

	c.Check(err, gc.ErrorMatches, "unrecognized args: .*")
}

func (s *trackSuite) TestInitEmptyName(c *gc.C) {
	err := s.trackCmd.Init([]string{
		"",
		"abc123",
	})

	c.Check(err, gc.ErrorMatches, "got empty name-or-id")
}

func (s *trackSuite) TestInitEmptyID(c *gc.C) {
	err := s.trackCmd.Init([]string{
		s.workload.Name,
		"",
	})

	c.Check(errors.Cause(err), gc.ErrorMatches, "unexpected end of JSON input")
}

func (s *trackSuite) TestInitMissingDetailsID(c *gc.C) {
	err := s.trackCmd.Init([]string{
		s.workload.Name,
		`{"status":{"state":"okay"}}`,
	})

	c.Check(err, jc.Satisfies, errors.IsNotValid)
}

func (s *trackSuite) TestInitMissingDetailsStatus(c *gc.C) {
	err := s.trackCmd.Init([]string{
		s.workload.Name,
		`{"id":"abc123"}`,
	})

	c.Check(err, jc.Satisfies, errors.IsNotValid)
}

func (s *trackSuite) TestInitBadJSON(c *gc.C) {
	err := s.trackCmd.Init([]string{
		s.workload.Name,
		`{"id":"abc123", "status":{"state":"okay"}`,
	})

	c.Check(errors.Cause(err), gc.ErrorMatches, "unexpected end of JSON input")
}

func (s *trackSuite) TestOverridesWithoutSubfield(c *gc.C) {
	s.workload.Workload.Description = "notFoo"
	s.setMetadata(s.workload)
	s.trackCmd.Overrides = []string{
		"description:foo",
	}
	s.init(c, s.workload.Name, "abc123-override", "okay")

	def := s.workload.Workload.Copy()
	def.Description = "foo"
	s.checkRunInfo(c, s.workload, workload.Info{
		Workload: def,
		Status:   workload.Status{State: workload.StateRunning},
		Details:  s.details,
	})
}

func (s *trackSuite) TestOverridesWithSubfield(c *gc.C) {
	s.workload.Workload.EnvVars = map[string]string{"foo": "bar"}
	s.setMetadata(s.workload)
	s.trackCmd.Overrides = []string{
		"env/foo:baz",
	}
	s.init(c, s.workload.Name, "abc123-override", "okay")

	def := s.workload.Workload.Copy()
	def.EnvVars = map[string]string{"foo": "baz"}
	s.checkRunInfo(c, s.workload, workload.Info{
		Workload: def,
		Status:   workload.Status{State: workload.StateRunning},
		Details:  s.details,
	})
}

func (s *trackSuite) TestOverridesMissingField(c *gc.C) {
	s.setMetadata(s.workload)
	s.trackCmd.Overrides = []string{
		":value",
	}
	s.init(c, s.workload.Name, "abc123-override", "okay")

	err := s.cmd.Run(s.cmdCtx)

	c.Check(err, gc.ErrorMatches, "override: missing field")
}

func (s *trackSuite) TestOverridesMissingValue(c *gc.C) {
	s.setMetadata(s.workload)
	s.trackCmd.Overrides = []string{
		"field:",
	}
	s.init(c, s.workload.Name, "abc123-override", "okay")

	err := s.cmd.Run(s.cmdCtx)

	c.Check(err, gc.ErrorMatches, "override: missing value")
}

func (s *trackSuite) TestOverridesMissingColon(c *gc.C) {
	s.setMetadata(s.workload)
	s.trackCmd.Overrides = []string{
		"fieldvalue",
	}
	s.init(c, s.workload.Name, "abc123-override", "okay")

	err := s.cmd.Run(s.cmdCtx)

	c.Check(err, gc.ErrorMatches, "override: missing value")
}

func (s *trackSuite) TestAdditionsWithoutSubfield(c *gc.C) {
	s.workload.Workload.Description = ""
	s.setMetadata(s.workload)
	s.trackCmd.Additions = []string{
		"description:foo",
	}
	s.init(c, s.workload.Name, "abc123-override", "okay")

	def := s.workload.Workload.Copy()
	def.Description = "foo"
	s.checkRunInfo(c, s.workload, workload.Info{
		Workload: def,
		Status:   workload.Status{State: workload.StateRunning},
		Details:  s.details,
	})
}

func (s *trackSuite) TestAdditionsWithSubfield(c *gc.C) {
	s.setMetadata(s.workload)
	s.trackCmd.Additions = []string{
		"env/foo:baz",
	}
	s.init(c, s.workload.Name, "abc123-override", "okay")

	def := s.workload.Workload.Copy()
	def.EnvVars = map[string]string{"foo": "baz"}
	s.checkRunInfo(c, s.workload, workload.Info{
		Workload: def,
		Status:   workload.Status{State: workload.StateRunning},
		Details:  s.details,
	})
}

func (s *trackSuite) TestAdditionsMissingField(c *gc.C) {
	s.setMetadata(s.workload)
	s.trackCmd.Additions = []string{
		":value",
	}
	s.init(c, s.workload.Name, "abc123-override", "okay")

	err := s.cmd.Run(s.cmdCtx)

	c.Check(err, gc.ErrorMatches, "extend: missing field")
}

func (s *trackSuite) TestAdditionsMissingValue(c *gc.C) {
	s.setMetadata(s.workload)
	s.trackCmd.Additions = []string{
		"field:",
	}
	s.init(c, s.workload.Name, "abc123-override", "okay")

	err := s.cmd.Run(s.cmdCtx)

	c.Check(err, gc.ErrorMatches, "extend: missing value")
}

func (s *trackSuite) TestAdditionMissingColon(c *gc.C) {
	s.setMetadata(s.workload)
	s.trackCmd.Additions = []string{
		"fieldvalue",
	}
	s.init(c, s.workload.Name, "abc123-override", "okay")

	err := s.cmd.Run(s.cmdCtx)

	c.Check(err, gc.ErrorMatches, "extend: missing value")
}

func (s *trackSuite) TestRunOkay(c *gc.C) {
	s.setMetadata(s.workload)
	s.init(c, s.workload.Name, "abc123", "running")

	s.checkRun(c, "", "")
	s.Stub.CheckCallNames(c, "List", "Plugin", "Track", "Flush")
}

func (s *trackSuite) TestRunAlreadyRegistered(c *gc.C) {
	s.workload.Details.ID = "xyz123"
	s.compCtx.workloads[s.workload.Name] = s.workload
	s.init(c, s.workload.Name, "abc123", "okay")

	err := s.trackCmd.Run(s.cmdCtx)

	c.Check(err, gc.ErrorMatches, ".*already tracked")
}

func (s *trackSuite) TestRunUpdatedWorkload(c *gc.C) {
	s.workload.Workload.Description = "bar"
	s.setMetadata(s.workload)
	s.trackCmd.Overrides = []string{"description:foo"}
	s.init(c, s.workload.Name, "abc123", "running")

	s.checkRun(c, "", "")

	s.workload.Workload = *s.trackCmd.UpdatedWorkload
	expected := s.workload
	expected.Status.State = workload.StateRunning
	expected.Details = s.details
	s.Stub.CheckCalls(c, []testing.StubCall{{
		FuncName: "List",
	}, {
		FuncName: "Plugin",
		Args:     []interface{}{&s.workload, ""},
	}, {
		FuncName: "Track",
		Args:     []interface{}{expected},
	}, {
		FuncName: "Flush",
		Args:     nil,
	}})
}
