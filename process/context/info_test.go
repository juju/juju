// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context_test

import (
	"strings"

	"github.com/juju/errors"
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
	meta = charm.Meta{
		Name:        "mycharm",
		Summary:     "a charm",
		Description: "a charm",
		Processes: map[string]charm.Process{
			"wistful": definition,
		},
	}
)

type infoSuite struct {
	commandSuite

	infoCmd *context.ProcInfoCommand
}

var _ = gc.Suite(&infoSuite{})

func (s *infoSuite) SetUpTest(c *gc.C) {
	s.commandSuite.SetUpTest(c)

	cmd, err := context.NewProcInfoCommand(s.Ctx)
	c.Assert(err, jc.ErrorIsNil)

	s.infoCmd = cmd
	s.setCommand(c, "info", s.infoCmd)

	cmd.ReadMetadata = s.readMetadata
}

func (s *infoSuite) init(c *gc.C, name string) {
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
usage: info [options] [<name>]
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
	context.AddProcs(s.compCtx, s.proc)

	err := s.infoCmd.Init([]string{s.proc.Name})
	c.Assert(err, jc.ErrorIsNil)

	c.Check(s.infoCmd.Name, gc.Equals, s.proc.Name)
}

func (s *infoSuite) TestInitWithNameAvailable(c *gc.C) {
	s.infoCmd.Available = true
	context.AddProcs(s.compCtx, s.proc)

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
	compCtx := newStubContextComponent(s.Stub)
	compCtx.procs["myprocess0"] = procs[0]
	compCtx.procs["myprocess1"] = procs[1]
	compCtx.procs["myprocess2"] = procs[2]
	s.init(c, "myprocess0")
	context.SetComponent(s.cmd, compCtx)

	s.checkRun(c, rawProcs[0]+"\n", "")
	s.Stub.CheckCallNames(c, "Get")
}

func (s *infoSuite) TestRunWithoutNameOkay(c *gc.C) {
	compCtx := newStubContextComponent(s.Stub)
	compCtx.procs["myprocess0"] = procs[0]
	compCtx.procs["myprocess1"] = procs[1]
	compCtx.procs["myprocess2"] = procs[2]
	context.AddProcs(s.compCtx, procs...)
	s.init(c, "")
	context.SetComponent(s.cmd, compCtx)

	expected := strings.Join(rawProcs, "\n")
	s.checkRun(c, expected+"\n", "")
	s.Stub.CheckCallNames(c, "List", "Get", "Get", "Get")
}

func (s *infoSuite) TestRunWithNameMissing(c *gc.C) {
	compCtx := newStubContextComponent(s.Stub)
	s.init(c, "myprocess0")
	context.SetComponent(s.cmd, compCtx)

	err := s.infoCmd.Run(s.cmdCtx)

	c.Check(err, jc.Satisfies, errors.IsNotFound)
	s.Stub.CheckCallNames(c, "Get")
}

func (s *infoSuite) TestRunWithoutNameEmpty(c *gc.C) {
	compCtx := newStubContextComponent(s.Stub)
	s.init(c, "")
	context.SetComponent(s.cmd, compCtx)

	s.checkRun(c, "", " [no processes registered]\n")
	s.Stub.CheckCallNames(c, "List")
}

func (s *infoSuite) TestRunWithNameAvailable(c *gc.C) {
	s.infoCmd.Available = true
	//context.AddProcs(s.compCtx, procs...)
	s.metadata = &meta
	s.init(c, "wistful")

	s.checkRun(c, rawDefinition+"\n", "")
	s.Stub.CheckCalls(c, nil)
}

func (s *infoSuite) TestRunWithoutNameAvailable(c *gc.C) {
	s.infoCmd.Available = true
	//context.AddProcs(s.compCtx, procs...)
	s.metadata = &meta
	s.init(c, "")

	s.checkRun(c, rawDefinition+"\n", "")
	s.Stub.CheckCalls(c, nil)
}

func (s *infoSuite) TestRunWithNameNotAvailable(c *gc.C) {
	s.infoCmd.Available = true
	//context.AddProcs(s.compCtx, procs...)
	s.init(c, "wistful")

	err := s.infoCmd.Run(s.cmdCtx)

	c.Check(err, jc.Satisfies, errors.IsNotFound)
	s.Stub.CheckCalls(c, nil)
}

func (s *infoSuite) TestRunWithoutNameNotAvailable(c *gc.C) {
	s.infoCmd.Available = true
	//context.AddProcs(s.compCtx, procs...)
	s.init(c, "")

	s.checkRun(c, "", " [no processes defined in charm]\n")
	s.Stub.CheckCalls(c, nil)
}
