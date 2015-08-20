// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v5"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/workload"
	"github.com/juju/juju/workload/context"
)

var (
	workload0 = workload.Info{
		Workload: charm.Workload{
			Name: "myworkload0",
			Type: "myplugin",
			TypeOptions: map[string]string{
				"extra": "5",
			},
			Command: "do-something",
			Image:   "myimage",
			EnvVars: map[string]string{
				"ENV_VAR": "some value",
			},
		},
		Details: workload.Details{
			ID: "xyz123",
			Status: workload.PluginStatus{
				State: "running",
			},
		},
	}
	workload1 = workload.Info{
		Workload: charm.Workload{
			Name:    "myworkload1",
			Type:    "myplugin",
			Command: "do-something",
			Image:   "myimage",
		},
		Details: workload.Details{
			ID: "xyz456",
			Status: workload.PluginStatus{
				State: "running",
			},
		},
	}
	workload2 = workload.Info{
		Workload: charm.Workload{
			Name: "myworkload2",
			Type: "myplugin",
		},
		Details: workload.Details{
			ID: "xyz789",
			Status: workload.PluginStatus{
				State: "invalid",
			},
		},
	}
)

type infoSuite struct {
	registeringCommandSuite

	infoCmd *context.WorkloadInfoCommand
}

var _ = gc.Suite(&infoSuite{})

func (s *infoSuite) SetUpTest(c *gc.C) {
	s.registeringCommandSuite.SetUpTest(c)

	cmd, err := context.NewWorkloadInfoCommand(s.Ctx)
	c.Assert(err, jc.ErrorIsNil)

	s.infoCmd = cmd
	s.setCommand(c, "workload-info", s.infoCmd)
}

func (s *infoSuite) init(c *gc.C, name string) {
	s.infoCmd.SetFlags(&gnuflag.FlagSet{})

	if name == "" {
		err := s.infoCmd.Init(nil)
		c.Assert(err, jc.ErrorIsNil)
	} else {
		err := s.infoCmd.Init([]string{name})
		c.Assert(err, jc.ErrorIsNil)
	}
}

func (s *infoSuite) TestCommandRegistered(c *gc.C) {
	s.checkCommandRegistered(c)
}

func (s *infoSuite) TestHelp(c *gc.C) {
	s.checkHelp(c, `
usage: workload-info [options] [<name-or-id>]
purpose: get info about a workload (or all of them)

options:
--format  (= yaml)
    specify output format (json|yaml)
-o, --output (= "")
    specify an output file

"workload-info" is used while a hook is running to access a currently
tracked workload (or the list of all the unit's workloads).
The workload info is printed to stdout as YAML-formatted text.
`[1:])
}

func (s *infoSuite) TestInitWithName(c *gc.C) {
	s.compCtx.workloads[s.workload.Name] = s.workload

	err := s.infoCmd.Init([]string{s.workload.Name})
	c.Assert(err, jc.ErrorIsNil)

	c.Check(s.infoCmd.Name, gc.Equals, s.workload.Name)
}

func (s *infoSuite) TestInitWithoutName(c *gc.C) {
	err := s.infoCmd.Init(nil)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(s.infoCmd.Name, gc.Equals, "")
}

func (s *infoSuite) TestInitNotFound(c *gc.C) {
	err := s.infoCmd.Init([]string{s.workload.Name})
	c.Assert(err, jc.ErrorIsNil)

	c.Check(s.infoCmd.Name, gc.Equals, s.workload.Name)
}

func (s *infoSuite) TestInitTooManyArgs(c *gc.C) {
	err := s.infoCmd.Init([]string{s.workload.Name, "other"})

	c.Check(err, gc.ErrorMatches, `unrecognized args: .*`)
}

func (s *infoSuite) TestRunWithIDkay(c *gc.C) {
	s.compCtx.workloads["myworkload0/xyz123"] = workload0
	s.compCtx.workloads["myworkload1/xyz456"] = workload1
	s.compCtx.workloads["myworkload2/xyz789"] = workload2
	s.init(c, "myworkload0/xyz123")

	expected := `
myworkload0/xyz123:
  workload:
    name: myworkload0
    description: ""
    type: myplugin
    typeoptions:
      extra: "5"
    command: do-something
    image: myimage
    ports: []
    volumes: []
    envvars:
      ENV_VAR: some value
  status:
    state: ""
    blocker: ""
    message: ""
  details:
    id: xyz123
    status:
      state: running
`[1:]
	s.checkRun(c, expected, "")
	s.Stub.CheckCallNames(c, "Get")
}

func (s *infoSuite) TestRunWithoutIDOkay(c *gc.C) {
	s.compCtx.workloads["myworkload0/xyz123"] = workload0
	s.compCtx.workloads["myworkload1/xyz456"] = workload1
	s.compCtx.workloads["myworkload2/xyz789"] = workload2
	s.init(c, "")

	expected := `
myworkload0/xyz123:
  workload:
    name: myworkload0
    description: ""
    type: myplugin
    typeoptions:
      extra: "5"
    command: do-something
    image: myimage
    ports: []
    volumes: []
    envvars:
      ENV_VAR: some value
  status:
    state: ""
    blocker: ""
    message: ""
  details:
    id: xyz123
    status:
      state: running
myworkload1/xyz456:
  workload:
    name: myworkload1
    description: ""
    type: myplugin
    typeoptions: {}
    command: do-something
    image: myimage
    ports: []
    volumes: []
    envvars: {}
  status:
    state: ""
    blocker: ""
    message: ""
  details:
    id: xyz456
    status:
      state: running
myworkload2/xyz789:
  workload:
    name: myworkload2
    description: ""
    type: myplugin
    typeoptions: {}
    command: ""
    image: ""
    ports: []
    volumes: []
    envvars: {}
  status:
    state: ""
    blocker: ""
    message: ""
  details:
    id: xyz789
    status:
      state: invalid
`[1:]
	s.checkRun(c, expected, "")
	s.Stub.CheckCallNames(c, "List", "Get", "Get", "Get")
}

func (s *infoSuite) TestRunWithNameOkay(c *gc.C) {
	s.compCtx.workloads["myworkload0/xyz123"] = workload0
	s.compCtx.workloads["myworkload1/xyz456"] = workload1
	s.compCtx.workloads["myworkload2/xyz789"] = workload2
	s.init(c, "myworkload0")

	expected := `
myworkload0/xyz123:
  workload:
    name: myworkload0
    description: ""
    type: myplugin
    typeoptions:
      extra: "5"
    command: do-something
    image: myimage
    ports: []
    volumes: []
    envvars:
      ENV_VAR: some value
  status:
    state: ""
    blocker: ""
    message: ""
  details:
    id: xyz123
    status:
      state: running
`[1:]
	s.checkRun(c, expected, "")
	s.Stub.CheckCallNames(c, "List", "Get")
}

func (s *infoSuite) TestRunWithIDMissing(c *gc.C) {
	s.init(c, "myworkload0/xyz123")

	s.checkRun(c, `myworkload0/xyz123: <not found>`+"\n", "")
	s.Stub.CheckCallNames(c, "Get")
}

func (s *infoSuite) TestRunWithNameMissing(c *gc.C) {
	s.init(c, "myworkload0")

	s.checkRun(c, `myworkload0: <not found>`+"\n", "")
	s.Stub.CheckCallNames(c, "List", "Get")
}

func (s *infoSuite) TestRunWithoutIDEmpty(c *gc.C) {
	s.init(c, "")

	s.checkRun(c, "", "<no workloads tracked>\n")
	s.Stub.CheckCallNames(c, "List")
}
