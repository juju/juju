// Copyright 2015 Canonical Ltd.
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
	"github.com/juju/juju/rpc/params"
)

var _ = tc.Suite(&listCommandSuite{})

type listCommandSuite struct {
	testing.FakeJujuXDGDataHomeSuite
}

func (s *listCommandSuite) TestInit(c *tc.C) {
	cmd := s.listCommand(nil, nil)
	err := cmdtesting.InitCommand(cmd, nil)
	c.Check(err, jc.ErrorIsNil)

	err = cmdtesting.InitCommand(cmd, []string{"anything"})
	c.Check(err.Error(), tc.Equals, `unrecognized args: ["anything"]`)
}

func (*listCommandSuite) listCommand(api *mockListClient, err error) cmd.Command {
	store := jujuclienttesting.MinimalStore()
	return block.NewListCommandForTest(store, api, err)
}

func (s *listCommandSuite) TestListEmpty(c *tc.C) {
	ctx, err := cmdtesting.RunCommand(c, s.listCommand(&mockListClient{}, nil))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(ctx), tc.Equals, "No commands are currently disabled.\n")
}

func (s *listCommandSuite) TestListError(c *tc.C) {
	_, err := cmdtesting.RunCommand(c, s.listCommand(
		&mockListClient{err: errors.New("boom")}, nil))
	c.Assert(err, tc.ErrorMatches, "boom")
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
		modelBlocks: []params.ModelBlockInfo{
			{
				Name:     "controller",
				UUID:     "fake-uuid-1",
				OwnerTag: "user-admin",
				Blocks:   []string{"BlockDestroy", "BlockRemove"},
			}, {
				Name:     "model-a",
				UUID:     "fake-uuid-2",
				OwnerTag: "user-bob@external",
				Blocks:   []string{"BlockChange"},
			}, {
				Name:     "model-b",
				UUID:     "fake-uuid-3",
				OwnerTag: "user-charlie@external",
				Blocks:   []string{"BlockDestroy", "BlockChange"},
			},
		},
	}
}

func (s *listCommandSuite) TestList(c *tc.C) {
	cmd := s.listCommand(s.mock(), nil)
	ctx, err := cmdtesting.RunCommand(c, cmd)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(ctx), tc.Equals, "")
	c.Assert(cmdtesting.Stdout(ctx), tc.Equals, ""+
		"Disabled commands  Message\n"+
		"destroy-model      Sysadmins in control.\n"+
		"all                just temporary\n",
	)
}

func (s *listCommandSuite) TestListYAML(c *tc.C) {
	cmd := s.listCommand(s.mock(), nil)
	ctx, err := cmdtesting.RunCommand(c, cmd, "--format", "yaml")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(ctx), tc.Equals, ""+
		"- command-set: destroy-model\n"+
		"  message: Sysadmins in control.\n"+
		"- command-set: all\n"+
		"  message: just temporary\n",
	)
}

func (s *listCommandSuite) TestListJSONEmpty(c *tc.C) {
	ctx, err := cmdtesting.RunCommand(c, s.listCommand(&mockListClient{}, nil), "--format", "json")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(ctx), tc.Equals, "[]\n")
}

func (s *listCommandSuite) TestListJSON(c *tc.C) {
	cmd := s.listCommand(s.mock(), nil)
	ctx, err := cmdtesting.RunCommand(c, cmd, "--format", "json")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(ctx), tc.Equals, ""+
		`[{"command-set":"destroy-model","message":"Sysadmins in control."},`+
		`{"command-set":"all","message":"just temporary"}]`+"\n")
}

func (s *listCommandSuite) TestListAll(c *tc.C) {
	cmd := s.listCommand(s.mock(), nil)
	ctx, err := cmdtesting.RunCommand(c, cmd, "--all")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(ctx), tc.Equals, "")
	c.Assert(cmdtesting.Stdout(ctx), tc.Equals, ""+
		"Name        Model UUID   Owner             Disabled commands\n"+
		"controller  fake-uuid-1  admin             destroy-model, remove-object\n"+
		"model-a     fake-uuid-2  bob@external      all\n"+
		"model-b     fake-uuid-3  charlie@external  all, destroy-model\n")
}

func (s *listCommandSuite) TestListAllYAML(c *tc.C) {
	cmd := s.listCommand(s.mock(), nil)
	ctx, err := cmdtesting.RunCommand(c, cmd, "--format", "yaml", "--all")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(ctx), tc.Equals, ""+
		"- name: controller\n"+
		"  model-uuid: fake-uuid-1\n"+
		"  owner: admin\n"+
		"  disabled-commands:\n"+
		"  - destroy-model\n"+
		"  - remove-object\n"+
		"- name: model-a\n"+
		"  model-uuid: fake-uuid-2\n"+
		"  owner: bob@external\n"+
		"  disabled-commands:\n"+
		"  - all\n"+
		"- name: model-b\n"+
		"  model-uuid: fake-uuid-3\n"+
		"  owner: charlie@external\n"+
		"  disabled-commands:\n"+
		"  - all\n"+
		"  - destroy-model\n")
}

func (s *listCommandSuite) TestListAllJSON(c *tc.C) {
	cmd := s.listCommand(s.mock(), nil)
	ctx, err := cmdtesting.RunCommand(c, cmd, "--format", "json", "--all")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(ctx), tc.Equals, "["+
		`{"name":"controller","model-uuid":"fake-uuid-1","owner":"admin","disabled-commands":["destroy-model","remove-object"]},`+
		`{"name":"model-a","model-uuid":"fake-uuid-2","owner":"bob@external","disabled-commands":["all"]},`+
		`{"name":"model-b","model-uuid":"fake-uuid-3","owner":"charlie@external","disabled-commands":["all","destroy-model"]}`+
		"]\n")
}

type mockListClient struct {
	blocks      []params.Block
	modelBlocks []params.ModelBlockInfo
	err         error
}

func (c *mockListClient) Close() error {
	return nil
}

func (c *mockListClient) List(ctx context.Context) ([]params.Block, error) {
	return c.blocks, c.err
}

func (c *mockListClient) ListBlockedModels(context.Context) ([]params.ModelBlockInfo, error) {
	return c.modelBlocks, c.err
}
