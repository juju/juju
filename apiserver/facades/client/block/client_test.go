// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package block_test

import (
	"context"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade/facadetest"
	"github.com/juju/juju/apiserver/facades/client/block"
	"github.com/juju/juju/apiserver/testing"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

type blockSuite struct {
	// TODO(anastasiamac) mock to remove ApiServerSuite
	jujutesting.ApiServerSuite
	api *block.API
}

var _ = gc.Suite(&blockSuite{})

func (s *blockSuite) SetUpTest(c *gc.C) {
	s.ApiServerSuite.SetUpTest(c)

	var err error
	auth := testing.FakeAuthorizer{
		Tag:        jujutesting.AdminUser,
		Controller: true,
	}
	s.api, err = block.NewAPI(facadetest.ModelContext{
		State_:     s.ControllerModel(c).State(),
		Resources_: common.NewResources(),
		Auth_:      auth,
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *blockSuite) TestListBlockNoneExistent(c *gc.C) {
	s.assertBlockList(c, 0)
}

func (s *blockSuite) assertBlockList(c *gc.C, length int) {
	all, err := s.api.List(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(all.Results, gc.HasLen, length)
}

func (s *blockSuite) TestSwitchValidBlockOn(c *gc.C) {
	s.assertSwitchBlockOn(c, state.DestroyBlock.String(), "for TestSwitchValidBlockOn")
}

func (s *blockSuite) assertSwitchBlockOn(c *gc.C, blockType, msg string) {
	on := params.BlockSwitchParams{
		Type:    blockType,
		Message: msg,
	}
	err := s.api.SwitchBlockOn(context.Background(), on)
	c.Assert(err.Error, gc.IsNil)
	s.assertBlockList(c, 1)
}

func (s *blockSuite) TestSwitchInvalidBlockOn(c *gc.C) {
	on := params.BlockSwitchParams{
		Type:    "invalid_block_type",
		Message: "for TestSwitchInvalidBlockOn",
	}

	c.Assert(func() { s.api.SwitchBlockOn(context.Background(), on) }, gc.PanicMatches, ".*unknown block type.*")
}

func (s *blockSuite) TestSwitchBlockOff(c *gc.C) {
	valid := state.DestroyBlock
	s.assertSwitchBlockOn(c, valid.String(), "for TestSwitchBlockOff")

	off := params.BlockSwitchParams{
		Type: valid.String(),
	}
	err := s.api.SwitchBlockOff(context.Background(), off)
	c.Assert(err.Error, gc.IsNil)
	s.assertBlockList(c, 0)
}
