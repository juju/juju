// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package block_test

import (
	"github.com/juju/cmd/v4"
	"github.com/juju/cmd/v4/cmdtesting"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/block"
	"github.com/juju/juju/jujuclient/jujuclienttesting"
	"github.com/juju/juju/testing"
)

var _ = gc.Suite(&disableCommandSuite{})

type disableCommandSuite struct {
	testing.FakeJujuXDGDataHomeSuite
}

func (*disableCommandSuite) disableCommand(api *mockBlockClient, err error) cmd.Command {
	store := jujuclienttesting.MinimalStore()
	return block.NewDisableCommandForTest(store, api, err)
}

func (s *disableCommandSuite) TestInit(c *gc.C) {
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
			args: []string{"all", "lots", "of", "args"},
		},
	} {
		cmd := s.disableCommand(&mockBlockClient{}, nil)
		err := cmdtesting.InitCommand(cmd, test.args)
		if test.err == "" {
			c.Check(err, jc.ErrorIsNil)
		} else {
			c.Check(err.Error(), gc.Equals, test.err)
		}
	}
}

func (s *disableCommandSuite) TestRunGetAPIError(c *gc.C) {
	cmd := s.disableCommand(nil, errors.New("boom"))
	_, err := cmdtesting.RunCommand(c, cmd, "all")
	c.Assert(err.Error(), gc.Equals, "cannot connect to the API: boom")
}

func (s *disableCommandSuite) TestRun(c *gc.C) {
	for _, test := range []struct {
		args    []string
		type_   string
		message string
	}{{
		args:    []string{"all", "this is a single arg message"},
		type_:   "BlockChange",
		message: "this is a single arg message",
	}, {
		args:    []string{"destroy-model", "this", "is", "many", "args"},
		type_:   "BlockDestroy",
		message: "this is many args",
	}, {
		args:    []string{"remove-object", "this is a", "mix"},
		type_:   "BlockRemove",
		message: "this is a mix",
	}} {
		mockClient := &mockBlockClient{}
		cmd := s.disableCommand(mockClient, nil)
		_, err := cmdtesting.RunCommand(c, cmd, test.args...)
		c.Check(err, jc.ErrorIsNil)
		c.Check(mockClient.blockType, gc.Equals, test.type_)
		c.Check(mockClient.message, gc.Equals, test.message)
	}
}

func (s *disableCommandSuite) TestRunError(c *gc.C) {
	mockClient := &mockBlockClient{err: errors.New("boom")}
	cmd := s.disableCommand(mockClient, nil)
	_, err := cmdtesting.RunCommand(c, cmd, "all")
	c.Check(err, gc.ErrorMatches, "boom")
}

type mockBlockClient struct {
	blockType string
	message   string
	err       error
}

func (c *mockBlockClient) Close() error {
	return nil
}

func (c *mockBlockClient) SwitchBlockOn(blockType, message string) error {
	c.blockType = blockType
	c.message = message
	return c.err
}
