// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package block_test

import (
	"errors"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/juju/block"
	"github.com/juju/juju/testing"
)

var _ = gc.Suite(&listCommandSuite{})

type listCommandSuite struct {
	testing.FakeJujuXDGDataHomeSuite
}

func (s *listCommandSuite) TestInit(c *gc.C) {
	cmd := block.NewListCommand()
	err := testing.InitCommand(cmd, nil)
	c.Check(err, jc.ErrorIsNil)

	err = testing.InitCommand(cmd, []string{"anything"})
	c.Check(err.Error(), gc.Equals, `unrecognized args: ["anything"]`)
}

func (s *listCommandSuite) TestListEmpty(c *gc.C) {
	ctx, err := testing.RunCommand(c, block.NewListCommandForTest(&mockListClient{}, nil))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(testing.Stdout(ctx), gc.Equals, "No commands are currently disabled.\n")
}

func (s *listCommandSuite) TestListError(c *gc.C) {
	_, err := testing.RunCommand(c, block.NewListCommandForTest(
		&mockListClient{err: errors.New("boom")}, nil))
	c.Assert(err, gc.ErrorMatches, "boom")
}

func (s *listCommandSuite) mock() *mockListClient {
	return &mockListClient{
		blocks: []params.Block{
			{
				Type:    "BlockDestroy",
				Message: "Sysadmins in control.",
			}, {
				Type:    "BlockChange",
				Message: "just temporary",
			},
		},
	}
}

func (s *listCommandSuite) TestList(c *gc.C) {
	cmd := block.NewListCommandForTest(s.mock(), nil)
	ctx, err := testing.RunCommand(c, cmd)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(testing.Stderr(ctx), gc.Equals, "")
	c.Assert(testing.Stdout(ctx), gc.Equals, ""+
		"COMMANDS       MESSAGE\n"+
		"destroy-model  Sysadmins in control.\n"+
		"all            just temporary\n"+
		"\n",
	)
}

func (s *listCommandSuite) TestListYAML(c *gc.C) {
	cmd := block.NewListCommandForTest(s.mock(), nil)
	ctx, err := testing.RunCommand(c, cmd, "--format", "yaml")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(testing.Stdout(ctx), gc.Equals, ""+
		"- command-set: destroy-model\n"+
		"  message: Sysadmins in control.\n"+
		"- command-set: all\n"+
		"  message: just temporary\n",
	)
}

func (s *listCommandSuite) TestListJSON(c *gc.C) {
	cmd := block.NewListCommandForTest(s.mock(), nil)
	ctx, err := testing.RunCommand(c, cmd, "--format", "json")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(testing.Stdout(ctx), gc.Equals, ""+
		`[{"command-set":"destroy-model","message":"Sysadmins in control."},`+
		`{"command-set":"all","message":"just temporary"}]`+"\n")
}

type mockListClient struct {
	blocks []params.Block
	err    error
}

func (c *mockListClient) Close() error {
	return nil
}

func (c *mockListClient) List() ([]params.Block, error) {
	return c.blocks, c.err
}
