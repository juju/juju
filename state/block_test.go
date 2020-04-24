// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"strings"

	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"

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

func assertNoModelBlock(c *gc.C, st *state.State) {
	all, err := st.AllBlocks()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(all, gc.HasLen, 0)
}

func (s *blockSuite) TestNoInitialBlocks(c *gc.C) {
	assertNoModelBlock(c, s.State)
}

func (s *blockSuite) assertNoTypedBlock(c *gc.C, t state.BlockType) {
	one, found, err := s.State.GetBlockForType(t)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found, jc.IsFalse)
	c.Assert(one, gc.IsNil)
}

func (s *blockSuite) assertModelHasBlock(c *gc.C, st *state.State, t state.BlockType, msg string) {
	block, found, err := st.GetBlockForType(t)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found, jc.IsTrue)
	c.Assert(block, gc.NotNil)
	c.Assert(block.Type(), gc.Equals, t)
	tag, err := block.Tag()
	c.Assert(err, jc.ErrorIsNil)
	m, err := st.Model()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(tag, gc.Equals, m.ModelTag())
	c.Assert(block.Message(), gc.Equals, msg)
}

func (s *blockSuite) switchOnBlock(c *gc.C, t state.BlockType, message ...string) {
	m := strings.Join(message, " ")
	err := s.State.SwitchBlockOn(state.DestroyBlock, m)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *blockSuite) TestSwitchOnBlock(c *gc.C) {
	s.switchOnBlock(c, state.DestroyBlock, "some message")
	s.assertModelHasBlock(c, s.State, state.DestroyBlock, "some message")
}

func (s *blockSuite) TestSwitchOnBlockAlreadyOn(c *gc.C) {
	s.switchOnBlock(c, state.DestroyBlock, "first message")
	s.switchOnBlock(c, state.DestroyBlock, "second message")
	s.assertModelHasBlock(c, s.State, state.DestroyBlock, "second message")
}

func (s *blockSuite) switchOffBlock(c *gc.C, t state.BlockType) {
	err := s.State.SwitchBlockOff(t)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *blockSuite) TestSwitchOffBlockNoBlock(c *gc.C) {
	s.switchOffBlock(c, state.DestroyBlock)
	assertNoModelBlock(c, s.State)
	s.assertNoTypedBlock(c, state.DestroyBlock)
}

func (s *blockSuite) TestSwitchOffBlock(c *gc.C) {
	s.switchOnBlock(c, state.DestroyBlock)
	s.switchOffBlock(c, state.DestroyBlock)
	assertNoModelBlock(c, s.State)
	s.assertNoTypedBlock(c, state.DestroyBlock)
}

func (s *blockSuite) TestNonsenseBlocked(c *gc.C) {
	bType := state.BlockType(42)
	// This could be useful for entity blocks...
	s.switchOnBlock(c, bType)
	s.switchOffBlock(c, bType)
	// but for multiwatcher, it should panic.
	c.Assert(func() { bType.ToParams() }, gc.PanicMatches, ".*unknown block type.*")
}

func (s *blockSuite) TestMultiModelBlocked(c *gc.C) {
	// create another model
	_, st2 := s.createTestModel(c)
	defer st2.Close()

	// switch one block type on
	t := state.ChangeBlock
	msg := "another model tst"
	err := st2.SwitchBlockOn(t, msg)
	c.Assert(err, jc.ErrorIsNil)
	s.assertModelHasBlock(c, st2, t, msg)

	//check correct model has it
	assertNoModelBlock(c, s.State)
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
	model, st, err := s.Controller.NewModel(state.ModelArgs{
		Type:                    state.ModelTypeIAAS,
		CloudName:               "dummy",
		CloudRegion:             "dummy-region",
		Config:                  cfg,
		Owner:                   owner,
		StorageProviderRegistry: storage.StaticProviderRegistry{},
	})
	c.Assert(err, jc.ErrorIsNil)
	return model, st
}

func (s *blockSuite) TestConcurrentBlocked(c *gc.C) {
	switchBlockOn := func() {
		msg := ""
		t := state.DestroyBlock
		err := s.State.SwitchBlockOn(t, msg)
		c.Assert(err, jc.ErrorIsNil)
		s.assertModelHasBlock(c, s.State, t, msg)
	}
	defer state.SetBeforeHooks(c, s.State, switchBlockOn).Check()
	msg := "concurrency tst"
	t := state.RemoveBlock
	err := s.State.SwitchBlockOn(t, msg)
	c.Assert(err, jc.ErrorIsNil)
	s.assertModelHasBlock(c, s.State, t, msg)
}
