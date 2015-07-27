// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v5"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/process"
	"github.com/juju/juju/process/context"
)

var (
	proc0 = process.Info{
		Process: charm.Process{
			Name: "myprocess0",
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
		Details: process.Details{
			ID: "xyz123",
			Status: process.PluginStatus{
				Label: "running",
			},
		},
	}
	proc1 = process.Info{
		Process: charm.Process{
			Name:    "myprocess1",
			Type:    "myplugin",
			Command: "do-something",
			Image:   "myimage",
		},
		Details: process.Details{
			ID: "xyz456",
			Status: process.PluginStatus{
				Label: "running",
			},
		},
	}
	proc2 = process.Info{
		Process: charm.Process{
			Name: "myprocess2",
			Type: "myplugin",
		},
		Details: process.Details{
			ID: "xyz789",
			Status: process.PluginStatus{
				Label: "invalid",
			},
		},
	}
)

type infoSuite struct {
	registeringCommandSuite

	infoCmd *context.ProcInfoCommand
}

var _ = gc.Suite(&infoSuite{})

func (s *infoSuite) SetUpTest(c *gc.C) {
	s.registeringCommandSuite.SetUpTest(c)

	cmd, err := context.NewProcInfoCommand(s.Ctx)
	c.Assert(err, jc.ErrorIsNil)

	s.infoCmd = cmd
	s.setCommand(c, "process-info", s.infoCmd)
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
usage: process-info [options] [<name>]
purpose: get info about a workload process (or all of them)

options:
--format  (= yaml)
    specify output format (json|yaml)
-o, --output (= "")
    specify an output file

"process-info" is used while a hook is running to access a currently
registered workload process (or the list of all the unit's processes).
The process info is printed to stdout as YAML-formatted text.
`[1:])
}

func (s *infoSuite) TestInitWithName(c *gc.C) {
	s.compCtx.procs[s.proc.Name] = s.proc

	err := s.infoCmd.Init([]string{s.proc.Name})
	c.Assert(err, jc.ErrorIsNil)

	c.Check(s.infoCmd.Name, gc.Equals, s.proc.Name)
}

func (s *infoSuite) TestInitWithoutName(c *gc.C) {
	err := s.infoCmd.Init(nil)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(s.infoCmd.Name, gc.Equals, "")
}

func (s *infoSuite) TestInitNotFound(c *gc.C) {
	err := s.infoCmd.Init([]string{s.proc.Name})
	c.Assert(err, jc.ErrorIsNil)

	c.Check(s.infoCmd.Name, gc.Equals, s.proc.Name)
}

func (s *infoSuite) TestInitTooManyArgs(c *gc.C) {
	err := s.infoCmd.Init([]string{s.proc.Name, "other"})

	c.Check(err, gc.ErrorMatches, `unrecognized args: .*`)
}

func (s *infoSuite) TestRunWithIDkay(c *gc.C) {
	s.compCtx.procs["myprocess0/xyz123"] = &proc0
	s.compCtx.procs["myprocess1/xyz456"] = &proc1
	s.compCtx.procs["myprocess2/xyz789"] = &proc2
	s.init(c, "myprocess0/xyz123")

	expected := `
myprocess0/xyz123:
  process:
    name: myprocess0
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
  details:
    id: xyz123
    status:
      label: running
`[1:]
	s.checkRun(c, expected, "")
	s.Stub.CheckCallNames(c, "Get")
}

func (s *infoSuite) TestRunWithoutIDOkay(c *gc.C) {
	s.compCtx.procs["myprocess0/xyz123"] = &proc0
	s.compCtx.procs["myprocess1/xyz456"] = &proc1
	s.compCtx.procs["myprocess2/xyz789"] = &proc2
	s.init(c, "")

	expected := `
myprocess0/xyz123:
  process:
    name: myprocess0
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
  details:
    id: xyz123
    status:
      label: running
myprocess1/xyz456:
  process:
    name: myprocess1
    description: ""
    type: myplugin
    typeoptions: {}
    command: do-something
    image: myimage
    ports: []
    volumes: []
    envvars: {}
  details:
    id: xyz456
    status:
      label: running
myprocess2/xyz789:
  process:
    name: myprocess2
    description: ""
    type: myplugin
    typeoptions: {}
    command: ""
    image: ""
    ports: []
    volumes: []
    envvars: {}
  details:
    id: xyz789
    status:
      label: invalid
`[1:]
	s.checkRun(c, expected, "")
	s.Stub.CheckCallNames(c, "List", "Get", "Get", "Get")
}

func (s *infoSuite) TestRunWithNameOkay(c *gc.C) {
	s.compCtx.procs["myprocess0/xyz123"] = &proc0
	s.compCtx.procs["myprocess1/xyz456"] = &proc1
	s.compCtx.procs["myprocess2/xyz789"] = &proc2
	s.init(c, "myprocess0")

	expected := `
myprocess0/xyz123:
  process:
    name: myprocess0
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
  details:
    id: xyz123
    status:
      label: running
`[1:]
	s.checkRun(c, expected, "")
	s.Stub.CheckCallNames(c, "List", "Get")
}

func (s *infoSuite) TestRunWithIDMissing(c *gc.C) {
	s.init(c, "myprocess0/xyz123")

	s.checkRun(c, `myprocess0/xyz123: <not found>`+"\n", "")
	s.Stub.CheckCallNames(c, "Get")
}

func (s *infoSuite) TestRunWithNameMissing(c *gc.C) {
	s.init(c, "myprocess0")

	s.checkRun(c, `myprocess0: <not found>`+"\n", "")
	s.Stub.CheckCallNames(c, "List", "Get")
}

func (s *infoSuite) TestRunWithoutIDEmpty(c *gc.C) {
	s.init(c, "")

	s.checkRun(c, "", "<no processes registered>\n")
	s.Stub.CheckCallNames(c, "List")
}
