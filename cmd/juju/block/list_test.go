// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package block_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/block"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/state/multiwatcher"
	"github.com/juju/juju/testing"
)

type listCommandSuite struct {
	ProtectionCommandSuite
	mockClient *block.MockBlockClient
}

func (s *listCommandSuite) SetUpTest(c *gc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)
	s.mockClient = &block.MockBlockClient{}
	s.PatchValue(block.ListClient, func(_ *modelcmd.ModelCommandBase) (block.BlockListAPI, error) {
		return s.mockClient, nil
	})
}

var _ = gc.Suite(&listCommandSuite{})

func (s *listCommandSuite) TestListEmpty(c *gc.C) {
	ctx, err := testing.RunCommand(c, block.NewListCommand())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(testing.Stdout(ctx), gc.Equals, `
destroy-model  =off
remove-object  =off
all-changes    =off
`)
}

func (s *listCommandSuite) TestList(c *gc.C) {
	s.mockClient.SwitchBlockOn(string(multiwatcher.BlockRemove), "Test this one")
	ctx, err := testing.RunCommand(c, block.NewListCommand())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(testing.Stdout(ctx), gc.Equals, `
destroy-model  =off
remove-object  =on, Test this one
all-changes    =off
`)
}

func (s *listCommandSuite) TestListYaml(c *gc.C) {
	s.mockClient.SwitchBlockOn(string(multiwatcher.BlockRemove), "Test this one")
	ctx, err := testing.RunCommand(c, block.NewListCommand(), "--format", "yaml")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(testing.Stdout(ctx), gc.Equals, `
- block: destroy-model
  enabled: false
- block: remove-object
  enabled: true
  message: Test this one
- block: all-changes
  enabled: false
`[1:])
}

func (s *listCommandSuite) TestListJson(c *gc.C) {
	s.mockClient.SwitchBlockOn(string(multiwatcher.BlockRemove), "Test this one")
	ctx, err := testing.RunCommand(c, block.NewListCommand(), "--format", "json")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(testing.Stdout(ctx), gc.Equals, `[{"block":"destroy-model","enabled":false},{"block":"remove-object","enabled":true,"message":"Test this one"},{"block":"all-changes","enabled":false}]
`)
}
