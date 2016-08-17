// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"fmt"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/state"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/testing"
)

type blockSuite struct {
	ConnSuite
}

var _ = gc.Suite(&blockSuite{})

func (s *blockSuite) SetUpTest(c *gc.C) {
	s.ConnSuite.SetUpTest(c)
}

func assertNoEnvBlock(c *gc.C, st *state.State) {
	all, err := st.AllBlocks()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(all, gc.HasLen, 0)
}

func (s *blockSuite) assertNoTypedBlock(c *gc.C, t state.BlockType) {
	one, found, err := s.State.GetBlockForType(t)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found, jc.IsFalse)
	c.Assert(one, gc.IsNil)
}

func assertEnvHasBlock(c *gc.C, st *state.State, t state.BlockType, msg string) {
	dBlock, found, err := st.GetBlockForType(t)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found, jc.IsTrue)
	c.Assert(dBlock, gc.NotNil)
	c.Assert(dBlock.Type(), gc.DeepEquals, t)
	tag, err := dBlock.Tag()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(tag, gc.DeepEquals, st.ModelTag())
	c.Assert(dBlock.Message(), gc.DeepEquals, msg)
}

func (s *blockSuite) switchOnBlock(c *gc.C, t state.BlockType) string {
	msg := ""
	err := s.State.SwitchBlockOn(t, msg)
	c.Assert(err, jc.ErrorIsNil)

	assertEnvHasBlock(c, s.State, t, msg)
	return msg
}

func (s *blockSuite) switchOffBlock(c *gc.C, t state.BlockType) {
	err := s.State.SwitchBlockOff(t)
	c.Assert(err, jc.ErrorIsNil)
	assertNoEnvBlock(c, s.State)
	s.assertNoTypedBlock(c, t)
}

func (s *blockSuite) assertBlocked(c *gc.C, t state.BlockType) {
	msg := s.switchOnBlock(c, t)

	expectedErr := fmt.Sprintf(".*block %v is already ON.*", t.String())
	// cannot duplicate
	err := s.State.SwitchBlockOn(t, msg)
	c.Assert(errors.Cause(err), gc.ErrorMatches, expectedErr)

	// cannot update
	err = s.State.SwitchBlockOn(t, "Test block update")
	c.Assert(errors.Cause(err), gc.ErrorMatches, expectedErr)

	s.switchOffBlock(c, t)

	err = s.State.SwitchBlockOff(t)
	expectedErr = fmt.Sprintf(".*block %v is already OFF.*", t.String())
	c.Assert(errors.Cause(err), gc.ErrorMatches, expectedErr)
}

func (s *blockSuite) TestNewModelNotBlocked(c *gc.C) {
	assertNoEnvBlock(c, s.State)
	s.assertNoTypedBlock(c, state.DestroyBlock)
	s.assertNoTypedBlock(c, state.RemoveBlock)
	s.assertNoTypedBlock(c, state.ChangeBlock)
}

func (s *blockSuite) TestDestroyBlocked(c *gc.C) {
	s.assertBlocked(c, state.DestroyBlock)
}

func (s *blockSuite) TestRemoveBlocked(c *gc.C) {
	s.assertBlocked(c, state.RemoveBlock)
}

func (s *blockSuite) TestChangeBlocked(c *gc.C) {
	s.assertBlocked(c, state.ChangeBlock)
}

func (s *blockSuite) TestNonsenseBlocked(c *gc.C) {
	bType := state.BlockType(42)
	// This could be useful for entity blocks...
	s.switchOnBlock(c, bType)
	s.switchOffBlock(c, bType)
	// but for multiwatcher, it should panic.
	c.Assert(func() { bType.ToParams() }, gc.PanicMatches, ".*unknown block type.*")
}

func (s *blockSuite) TestMultiEnvBlocked(c *gc.C) {
	// create another env
	_, st2 := s.createTestModel(c)
	defer st2.Close()

	// switch one block type on
	t := state.ChangeBlock
	msg := "another env tst"
	err := st2.SwitchBlockOn(t, msg)
	c.Assert(err, jc.ErrorIsNil)
	assertEnvHasBlock(c, st2, t, msg)

	//check correct env has it
	assertNoEnvBlock(c, s.State)
	s.assertNoTypedBlock(c, t)
}

func (s *blockSuite) TestAllBlocksForController(c *gc.C) {
	_, st2 := s.createTestModel(c)
	defer st2.Close()

	err := st2.SwitchBlockOn(state.ChangeBlock, "block test")
	c.Assert(err, jc.ErrorIsNil)
	err = s.State.SwitchBlockOn(state.ChangeBlock, "block test")
	c.Assert(err, jc.ErrorIsNil)

	blocks, err := s.State.AllBlocksForController()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(blocks), gc.Equals, 2)
}

func (s *blockSuite) TestRemoveAllBlocksForController(c *gc.C) {
	_, st2 := s.createTestModel(c)
	defer st2.Close()

	err := st2.SwitchBlockOn(state.ChangeBlock, "block test")
	c.Assert(err, jc.ErrorIsNil)
	err = s.State.SwitchBlockOn(state.ChangeBlock, "block test")
	c.Assert(err, jc.ErrorIsNil)

	err = s.State.RemoveAllBlocksForController()
	c.Assert(err, jc.ErrorIsNil)

	blocks, err := s.State.AllBlocksForController()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(blocks), gc.Equals, 0)
}

func (s *blockSuite) TestRemoveAllBlocksForControllerNoBlocks(c *gc.C) {
	_, st2 := s.createTestModel(c)
	defer st2.Close()

	err := st2.RemoveAllBlocksForController()
	c.Assert(err, jc.ErrorIsNil)

	blocks, err := st2.AllBlocksForController()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(blocks), gc.Equals, 0)
}

func (s *blockSuite) TestModelUUID(c *gc.C) {
	st := s.Factory.MakeModel(c, nil)
	defer st.Close()
	err := st.SwitchBlockOn(state.ChangeBlock, "blocktest")
	c.Assert(err, jc.ErrorIsNil)

	blocks, err := st.AllBlocks()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(blocks), gc.Equals, 1)
	c.Assert(blocks[0].ModelUUID(), gc.Equals, st.ModelUUID())
}

func (s *blockSuite) createTestModel(c *gc.C) (*state.Model, *state.State) {
	uuid, err := utils.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	cfg := testing.CustomModelConfig(c, testing.Attrs{
		"name": "testing",
		"uuid": uuid.String(),
	})
	owner := names.NewUserTag("test@remote")
	env, st, err := s.State.NewModel(state.ModelArgs{
		CloudName: "dummy", CloudRegion: "dummy-region", Config: cfg, Owner: owner,
		StorageProviderRegistry: storage.StaticProviderRegistry{},
	})
	c.Assert(err, jc.ErrorIsNil)
	return env, st
}

func (s *blockSuite) TestConcurrentBlocked(c *gc.C) {
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
