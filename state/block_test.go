// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"github.com/juju/errors"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/state"
	"github.com/juju/juju/testing"
)

type BlockSuite struct {
	ConnSuite
}

var _ = gc.Suite(&BlockSuite{})

func (s *BlockSuite) SetUpTest(c *gc.C) {
	s.ConnSuite.SetUpTest(c)
}

func (s *BlockSuite) assertNoEnvBlock(c *gc.C) {
	all, err := state.GetEnvironmentBlocks(s.State)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(all, gc.HasLen, 0)
}

func (s *BlockSuite) assertNoTypedBlock(c *gc.C, t state.BlockType) {
	one, err := s.State.HasBlock(t)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(one, gc.IsNil)
}

func assertEnvHasBlock(c *gc.C, st *state.State, t state.BlockType, msg string) {
	dBlock, err := st.HasBlock(t)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(dBlock, gc.NotNil)
	c.Assert(dBlock.Type(), gc.DeepEquals, t)
	c.Assert(dBlock.Environment(), gc.DeepEquals, st.EnvironUUID())
	c.Assert(dBlock.Message(), gc.DeepEquals, msg)
}

func (s *BlockSuite) assertBlocked(c *gc.C, t state.BlockType) {
	msg := ""
	err := s.State.SwitchBlockOn(t, msg)
	c.Assert(err, jc.ErrorIsNil)

	// cannot duplicate
	err = s.State.SwitchBlockOn(t, msg)
	c.Assert(errors.Cause(err), gc.ErrorMatches, ".*block is already ON.*")

	// cannot update
	err = s.State.SwitchBlockOn(t, "Test block update")
	c.Assert(errors.Cause(err), gc.ErrorMatches, ".*block is already ON.*")

	assertEnvHasBlock(c, s.State, t, msg)

	err = s.State.SwitchBlockOff(t)
	c.Assert(err, jc.ErrorIsNil)
	s.assertNoEnvBlock(c)
	s.assertNoTypedBlock(c, t)
}

func (s *BlockSuite) TestNewEnvironmentNotBlocked(c *gc.C) {
	s.assertNoEnvBlock(c)
	s.assertNoTypedBlock(c, state.DestroyBlock)
	s.assertNoTypedBlock(c, state.RemoveBlock)
	s.assertNoTypedBlock(c, state.ChangeBlock)
}

func (s *BlockSuite) TestDestroyBlocked(c *gc.C) {
	s.assertBlocked(c, state.DestroyBlock)
}

func (s *BlockSuite) TestRemoveBlocked(c *gc.C) {
	s.assertBlocked(c, state.RemoveBlock)
}

func (s *BlockSuite) TestChangeBlocked(c *gc.C) {
	s.assertBlocked(c, state.ChangeBlock)
}

func (s *BlockSuite) TestNonsenseBlocked(c *gc.C) {
	// This could be useful for entity blocks...
	// but is it valid now?
	s.assertBlocked(c, state.BlockType(42))
}

func (s *BlockSuite) TestMultiEnvBlocked(c *gc.C) {
	// create another env
	_, st2 := s.createTestEnv(c)
	defer st2.Close()

	// switch one block type on
	t := state.ChangeBlock
	msg := "another env tst"
	err := st2.SwitchBlockOn(t, msg)
	c.Assert(err, jc.ErrorIsNil)
	assertEnvHasBlock(c, st2, t, msg)

	//check correct env has it
	s.assertNoEnvBlock(c)
	s.assertNoTypedBlock(c, t)
}

func (s *BlockSuite) createTestEnv(c *gc.C) (*state.Environment, *state.State) {
	uuid, err := utils.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	cfg := testing.CustomEnvironConfig(c, testing.Attrs{
		"name": "testing",
		"uuid": uuid.String(),
	})
	owner := names.NewUserTag("test@remote")
	env, st, err := s.State.NewEnvironment(cfg, owner)
	c.Assert(err, jc.ErrorIsNil)
	return env, st
}

func (s *BlockSuite) TestConcurrentBlocked(c *gc.C) {
	switchBlockOn := func() {
		msg := ""
		t := state.DestroyBlock
		err := s.State.SwitchBlockOn(t, msg)
		c.Assert(err, jc.ErrorIsNil)
		assertEnvHasBlock(c, s.State, t, msg)
	}
	defer state.SetBeforeHooks(c, s.State, switchBlockOn).Check()
	msg := "concurrency tst"
	t := state.RemoveBlock
	err := s.State.SwitchBlockOn(t, msg)
	c.Assert(err, jc.ErrorIsNil)
	assertEnvHasBlock(c, s.State, t, msg)
}
