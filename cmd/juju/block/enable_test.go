// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package block_test

import (
	"context"
	"errors"

	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"

	"github.com/juju/juju/cmd/juju/block"
	"github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/internal/cmd/cmdtesting"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/jujuclient/jujuclienttesting"
)

var _ = tc.Suite(&enableCommandSuite{})

type enableCommandSuite struct {
	testing.FakeJujuXDGDataHomeSuite
}

func (*enableCommandSuite) enableCommand(api *mockUnblockClient, err error) cmd.Command {
	store := jujuclienttesting.MinimalStore()
	return block.NewEnableCommandForTest(store, api, err)
}

func (s *enableCommandSuite) TestInit(c *tc.C) {
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
		cmd := s.enableCommand(nil, nil)
		err := cmdtesting.InitCommand(cmd, test.args)
		if test.err == "" {
			c.Check(err, jc.ErrorIsNil)
		} else {
			c.Check(err.Error(), tc.Equals, test.err)
		}
	}
}

func (s *enableCommandSuite) TestRunGetAPIError(c *tc.C) {
	cmd := s.enableCommand(nil, errors.New("boom"))
	_, err := cmdtesting.RunCommand(c, cmd, "all")
	c.Assert(err.Error(), tc.Equals, "cannot connect to the API: boom")
}

func (s *enableCommandSuite) TestRun(c *tc.C) {
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
		cmd := s.enableCommand(mockClient, nil)
		_, err := cmdtesting.RunCommand(c, cmd, test.args...)
		c.Check(err, jc.ErrorIsNil)
		c.Check(mockClient.blockType, tc.Equals, test.type_)
	}
}

func (s *enableCommandSuite) TestRunError(c *tc.C) {
	mockClient := &mockUnblockClient{err: errors.New("boom")}
	cmd := s.enableCommand(mockClient, nil)
	_, err := cmdtesting.RunCommand(c, cmd, "all")
	c.Check(err, tc.ErrorMatches, "boom")
}

type mockUnblockClient struct {
	blockType string
	err       error
}

func (c *mockUnblockClient) Close() error {
	return nil
}

func (c *mockUnblockClient) SwitchBlockOff(ctx context.Context, blockType string) error {
	c.blockType = blockType
	return c.err
}
