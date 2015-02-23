// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"flag"
	"strings"
	stdtesting "testing"

	"github.com/juju/cmd"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/block"
	cmdblock "github.com/juju/juju/cmd/juju/block"
	cmdtesting "github.com/juju/juju/cmd/testing"
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

type CmdBlockSwitch struct {
	blockClient *block.Client
}

// NewCmdBlockSwitch creates a block switch used in testing
// to manage desired juju blocks.
func NewCmdBlockSwitch(st *api.State) CmdBlockSwitch {
	return CmdBlockSwitch{
		blockClient: block.NewClient(st),
	}
}

// on switches on desired block and
// asserts that no errors were encountered.
func (s *CmdBlockSwitch) on(c *gc.C, blockType, msg string) {
	c.Assert(s.blockClient.SwitchBlockOn(cmdblock.TranslateOperation(blockType), msg), gc.IsNil)
}

// BlockAllChanges switches changes block on.
// This prevents all changes to juju environment.
func (s *CmdBlockSwitch) BlockAllChanges(c *gc.C, msg string) {
	s.on(c, "all-changes", msg)
}

// BlockRemoveObject switches remove block on.
// This prevents any object/entity removal on juju environment
func (s *CmdBlockSwitch) BlockRemoveObject(c *gc.C, msg string) {
	s.on(c, "remove-object", msg)
}

// BlockDestroyEnvironment switches destory block on.
// This prevents juju environment destruction.
func (s *CmdBlockSwitch) BlockDestroyEnvironment(c *gc.C, msg string) {
	s.on(c, "destroy-environment", msg)
}

func (s *CmdBlockSwitch) AssertBlockError(c *gc.C, err error, msg string) {
	c.Assert(err, gc.ErrorMatches, cmd.ErrSilent.Error())
	// msg is logged
	stripped := strings.Replace(c.GetTestLog(), "\n", "", -1)
	c.Check(stripped, gc.Matches, msg)
}
