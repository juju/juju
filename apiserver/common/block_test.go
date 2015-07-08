// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"github.com/juju/errors"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing"
)

type mockBlock struct {
	state.Block
	t state.BlockType
	m string
}

func (m mockBlock) Id() string { return "" }

func (m mockBlock) Tag() (names.Tag, error) { return names.NewEnvironTag("mocktesting"), nil }

func (m mockBlock) Type() state.BlockType { return m.t }

func (m mockBlock) Message() string { return m.m }

func (m mockBlock) EnvUUID() string { return "" }

type blockCheckerSuite struct {
	testing.FakeJujuHomeSuite
	aBlock                  state.Block
	destroy, remove, change state.Block

	blockchecker *common.BlockChecker
}

var _ = gc.Suite(&blockCheckerSuite{})

func (s *blockCheckerSuite) SetUpTest(c *gc.C) {
	s.FakeJujuHomeSuite.SetUpTest(c)
	s.destroy = mockBlock{t: state.DestroyBlock, m: "Mock BLOCK testing: DESTROY"}
	s.remove = mockBlock{t: state.RemoveBlock, m: "Mock BLOCK testing: REMOVE"}
	s.change = mockBlock{t: state.ChangeBlock, m: "Mock BLOCK testing: CHANGE"}
	s.blockchecker = common.NewBlockChecker(s)
}

func (mock *blockCheckerSuite) GetBlockForType(t state.BlockType) (state.Block, bool, error) {
	if mock.aBlock.Type() == t {
		return mock.aBlock, true, nil
	} else {
		return nil, false, nil
	}
}

func (s *blockCheckerSuite) TestDestroyBlockChecker(c *gc.C) {
	s.aBlock = s.destroy
	s.assertErrorBlocked(c, true, s.blockchecker.DestroyAllowed(), s.destroy.Message())

	s.aBlock = s.remove
	s.assertErrorBlocked(c, true, s.blockchecker.DestroyAllowed(), s.remove.Message())

	s.aBlock = s.change
	s.assertErrorBlocked(c, true, s.blockchecker.DestroyAllowed(), s.change.Message())
}

func (s *blockCheckerSuite) TestRemoveBlockChecker(c *gc.C) {
	s.aBlock = s.destroy
	s.assertErrorBlocked(c, false, s.blockchecker.RemoveAllowed(), s.destroy.Message())

	s.aBlock = s.remove
	s.assertErrorBlocked(c, true, s.blockchecker.RemoveAllowed(), s.remove.Message())

	s.aBlock = s.change
	s.assertErrorBlocked(c, true, s.blockchecker.RemoveAllowed(), s.change.Message())
}

func (s *blockCheckerSuite) TestChangeBlockChecker(c *gc.C) {
	s.aBlock = s.destroy
	s.assertErrorBlocked(c, false, s.blockchecker.ChangeAllowed(), s.destroy.Message())

	s.aBlock = s.remove
	s.assertErrorBlocked(c, false, s.blockchecker.ChangeAllowed(), s.remove.Message())

	s.aBlock = s.change
	s.assertErrorBlocked(c, true, s.blockchecker.ChangeAllowed(), s.change.Message())
}

func (s *blockCheckerSuite) assertErrorBlocked(c *gc.C, blocked bool, err error, msg string) {
	if blocked {
		c.Assert(params.IsCodeOperationBlocked(err), jc.IsTrue)
		c.Assert(err, gc.ErrorMatches, msg)
	} else {
		c.Assert(errors.Cause(err), jc.ErrorIsNil)
	}
}
