// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package block_test

import (
	"errors"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/block"
	"github.com/juju/juju/testing"
)

var _ = gc.Suite(&enableCommandSuite{})

type enableCommandSuite struct {
	testing.FakeJujuXDGDataHomeSuite
}

func (s *enableCommandSuite) TestInit(c *gc.C) {
	for _, test := range []struct {
		args []string
		err  string
	}{
		{
			err: "missing command set (all, destroy-model, remove-object)",
		}, {
			args: []string{"other"},
			err:  "bad command set, valid options: all, destroy-model, remove-object",
		}, {
			args: []string{"all"},
		}, {
			args: []string{"destroy-model"},
		}, {
			args: []string{"remove-object"},
		}, {
			args: []string{"all", "extra"},
			err:  `unrecognized args: ["extra"]`,
		},
	} {
		cmd := block.NewEnableCommand()
		err := testing.InitCommand(cmd, test.args)
		if test.err == "" {
			c.Check(err, jc.ErrorIsNil)
		} else {
			c.Check(err.Error(), gc.Equals, test.err)
		}
	}
}

func (s *enableCommandSuite) TestRunGetAPIError(c *gc.C) {
	cmd := block.NewEnableCommandForTest(nil, errors.New("boom"))
	_, err := testing.RunCommand(c, cmd, "all")
	c.Assert(err.Error(), gc.Equals, "cannot connect to the API: boom")
}

func (s *enableCommandSuite) TestRun(c *gc.C) {
	for _, test := range []struct {
		args  []string
		type_ string
	}{{
		args:  []string{"all"},
		type_: "BlockChange",
	}, {
		args:  []string{"destroy-model"},
		type_: "BlockDestroy",
	}, {
		args:  []string{"remove-object"},
		type_: "BlockRemove",
	}} {
		mockClient := &mockUnblockClient{}
		cmd := block.NewEnableCommandForTest(mockClient, nil)
		_, err := testing.RunCommand(c, cmd, test.args...)
		c.Check(err, jc.ErrorIsNil)
		c.Check(mockClient.blockType, gc.Equals, test.type_)
	}
}

func (s *enableCommandSuite) TestRunError(c *gc.C) {
	mockClient := &mockUnblockClient{err: errors.New("boom")}
	cmd := block.NewEnableCommandForTest(mockClient, nil)
	_, err := testing.RunCommand(c, cmd, "all")
	c.Check(err, gc.ErrorMatches, "boom")
}

type mockUnblockClient struct {
	blockType string
	err       error
}

func (c *mockUnblockClient) Close() error {
	return nil
}

func (c *mockUnblockClient) SwitchBlockOff(blockType string) error {
	c.blockType = blockType
	return c.err
}
