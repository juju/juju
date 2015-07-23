// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context_test

import (
	"strings"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v5"

	"github.com/juju/juju/process"
	"github.com/juju/juju/process/context"
)

var (
	rawProcs = []string{
		`
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
`[1:],
		`
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
`[1:],
		`
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
`[1:],
	}
	procs = []*process.Info{{
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
	}, {
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
	}, {
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
	}}

	rawDefinition = `
name: wistful
description: ""
type: other-type
typeoptions: {}
command: run
image: ""
ports: []
volumes: []
envvars: {}
`[1:]
	definition = charm.Process{
		Name:    "wistful",
		Type:    "other-type",
		Command: "run",
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
	if name == "" {
		err := s.infoCmd.Init(nil)
		c.Assert(err, jc.ErrorIsNil)
	} else {
		err := s.infoCmd.Init([]string{name})
		c.Assert(err, jc.ErrorIsNil)
	}
	s.Stub.ResetCalls()
}

func (s *infoSuite) TestCommandRegistered(c *gc.C) {
	s.checkCommandRegistered(c)
}

func (s *infoSuite) TestHelp(c *gc.C) {
	s.checkHelp(c, `
usage: process-info [options] [<name>]
purpose: get info about a workload process (or all of them)

options:
--available  (= false)
    show unregistered processes instead

"info" is used while a hook is running to access a currently registered
workload process (or the list of all the unit's processes). The process
info is printed to stdout as YAML-formatted text.
`[1:])
}

func (s *infoSuite) TestInitWithNameRegistered(c *gc.C) {
	s.compCtx.procs[s.proc.Name] = s.proc

	err := s.infoCmd.Init([]string{s.proc.Name})
	c.Assert(err, jc.ErrorIsNil)

	c.Check(s.infoCmd.Name, gc.Equals, s.proc.Name)
}

func (s *infoSuite) TestInitWithNameAvailable(c *gc.C) {
	s.infoCmd.Available = true
	s.compCtx.procs[s.proc.Name] = s.proc

	err := s.infoCmd.Init([]string{s.proc.Name})
	c.Assert(err, jc.ErrorIsNil)

	c.Check(s.infoCmd.Name, gc.Equals, s.proc.Name)
}

func (s *infoSuite) TestInitWithoutNameRegistered(c *gc.C) {
	err := s.infoCmd.Init(nil)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(s.infoCmd.Name, gc.Equals, "")
}

func (s *infoSuite) TestInitWithoutNameAvailable(c *gc.C) {
	s.infoCmd.Available = true
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

	c.Check(err, gc.ErrorMatches, `expected <name> \(or nothing\), got: .*`)
}

func (s *infoSuite) TestRunWithNameOkay(c *gc.C) {
	s.compCtx.procs["myprocess0"] = procs[0]
	s.compCtx.procs["myprocess1"] = procs[1]
	s.compCtx.procs["myprocess2"] = procs[2]
	s.init(c, "myprocess0")

	s.checkRun(c, rawProcs[0]+"\n", "")
	s.Stub.CheckCallNames(c, "Get")
}

func (s *infoSuite) TestRunWithoutNameOkay(c *gc.C) {
	s.compCtx.procs["myprocess0"] = procs[0]
	s.compCtx.procs["myprocess1"] = procs[1]
	s.compCtx.procs["myprocess2"] = procs[2]
	s.init(c, "")

	expected := strings.Join(rawProcs, "\n")
	s.checkRun(c, expected+"\n", "")
	s.Stub.CheckCallNames(c, "List", "Get", "Get", "Get")
}

func (s *infoSuite) TestRunWithNameMissing(c *gc.C) {
	s.init(c, "myprocess0")

	s.checkRun(c, `["myprocess0" not found]`+"\n", "")
	s.Stub.CheckCallNames(c, "Get")
}

func (s *infoSuite) TestRunWithoutNameEmpty(c *gc.C) {
	s.init(c, "")

	s.checkRun(c, "", " [no processes registered]\n")
	s.Stub.CheckCallNames(c, "List")
}

func (s *infoSuite) TestRunWithNameAvailable(c *gc.C) {
	s.infoCmd.Available = true
	s.compCtx.definitions[definition.Name] = definition
	s.init(c, "wistful")

	s.checkRun(c, rawDefinition+"\n", "")
	s.Stub.CheckCallNames(c, "ListDefinitions")
}

func (s *infoSuite) TestRunWithoutNameAvailable(c *gc.C) {
	s.infoCmd.Available = true
	s.compCtx.definitions[definition.Name] = definition
	s.init(c, "")

	s.checkRun(c, rawDefinition+"\n", "")
	s.Stub.CheckCallNames(c, "ListDefinitions")
}

func (s *infoSuite) TestRunWithNameNotAvailable(c *gc.C) {
	s.infoCmd.Available = true
	s.init(c, "wistful")

	s.checkRun(c, `["wistful" not found]`+"\n", "")
	s.Stub.CheckCallNames(c, "ListDefinitions")
}

func (s *infoSuite) TestRunWithoutNameNotAvailable(c *gc.C) {
	s.infoCmd.Available = true
	s.init(c, "")

	s.checkRun(c, "", " [no processes defined in charm]\n")
	s.Stub.CheckCallNames(c, "ListDefinitions")
}
