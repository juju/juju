// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

// TODO(dimitern): bug http://pad.lv/1425569
// Disabled until we have time to fix these tests on i386 properly.
//
// +build !386

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
	_ "github.com/juju/juju/provider/dummy"
	"github.com/juju/juju/testing"
)

func TestPackage(t *stdtesting.T) {
	testing.MgoTestPackage(t)
}

// Reentrancy point for testing (something as close as possible to) the juju
// tool itself.
func TestRunMain(t *stdtesting.T) {
	if *cmdtesting.FlagRunMain {
		Main(flag.Args())
	}
}

type CmdBlockHelper struct {
	blockClient *block.Client
}

// NewCmdBlockHelper creates a block switch used in testing
// to manage desired juju blocks.
func NewCmdBlockHelper(st *api.State) CmdBlockHelper {
	return CmdBlockHelper{
		blockClient: block.NewClient(st),
	}
}

// on switches on desired block and
// asserts that no errors were encountered.
func (s *CmdBlockHelper) on(c *gc.C, blockType, msg string) {
	c.Assert(s.blockClient.SwitchBlockOn(cmdblock.TypeFromOperation(blockType), msg), gc.IsNil)
}

// BlockAllChanges switches changes block on.
// This prevents all changes to juju environment.
func (s *CmdBlockHelper) BlockAllChanges(c *gc.C, msg string) {
	s.on(c, "all-changes", msg)
}

// BlockRemoveObject switches remove block on.
// This prevents any object/entity removal on juju environment
func (s *CmdBlockHelper) BlockRemoveObject(c *gc.C, msg string) {
	s.on(c, "remove-object", msg)
}

// BlockDestroyEnvironment switches destroy block on.
// This prevents juju environment destruction.
func (s *CmdBlockHelper) BlockDestroyEnvironment(c *gc.C, msg string) {
	s.on(c, "destroy-environment", msg)
}

func (s *CmdBlockHelper) Close() {
	s.blockClient.Close()
}

func (s *CmdBlockHelper) AssertBlocked(c *gc.C, err error, msg string) {
	c.Assert(err, gc.ErrorMatches, cmd.ErrSilent.Error())
	// msg is logged
	stripped := strings.Replace(c.GetTestLog(), "\n", "", -1)
	c.Check(stripped, gc.Matches, msg)
}
