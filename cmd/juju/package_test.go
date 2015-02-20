// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"flag"
	stdtesting "testing"

	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/block"
	cmdblock "github.com/juju/juju/cmd/juju/block"
	cmdtesting "github.com/juju/juju/cmd/testing"
	jujutesting "github.com/juju/juju/juju/testing"
	_ "github.com/juju/juju/provider/dummy" // XXX Why?
	"github.com/juju/juju/testing"
)

func TestPackage(t *stdtesting.T) {
	testing.MgoTestPackage(t)
}

func badrun(c *gc.C, exit int, args ...string) string {
	args = append([]string{"juju"}, args...)
	return cmdtesting.BadRun(c, exit, args...)
}

// Reentrancy point for testing (something as close as possible to) the juju
// tool itself.
func TestRunMain(t *stdtesting.T) {
	if *cmdtesting.FlagRunMain {
		Main(flag.Args())
	}
}

type CmdBlockSuite struct {
	jujutesting.RepoSuite
	blockClient *block.Client
}

func (s *CmdBlockSuite) SetUpTest(c *gc.C) {
	s.RepoSuite.SetUpTest(c)
	s.blockClient = block.NewClient(s.APIState)
	c.Assert(s.blockClient, gc.NotNil)
}

// AssertSwitchBlockOn switches on desired block and
// asserts that no errors were encountered.
func (s *CmdBlockSuite) AssertSwitchBlockOn(c *gc.C, blockType, msg string) {
	c.Assert(s.blockClient.SwitchBlockOn(cmdblock.TranslateOperation(blockType), msg), gc.IsNil)
}
