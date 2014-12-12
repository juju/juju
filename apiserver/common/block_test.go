// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/testing"
)

type blocksSuite struct {
	testing.FakeJujuHomeSuite
	destroy, remove, change bool
	cfg                     *config.Config
}

var _ = gc.Suite(&blocksSuite{})

func (s *blocksSuite) TearDownTest(c *gc.C) {
	s.destroy, s.remove, s.change = false, false, false
}

func (s *blocksSuite) SetUpTest(c *gc.C) {
	s.FakeJujuHomeSuite.SetUpTest(c)
	cfg, err := config.New(
		config.UseDefaults,
		map[string]interface{}{
			"name": "block-env",
			"type": "any-type",
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	s.cfg = cfg
}

func (s *blocksSuite) TestBlockOperationErrorDestroy(c *gc.C) {
	// prevent destroy-environment
	s.blockDestroys(c)
	s.assertDestroyOperationBlocked(c, true)

	// prevent remove-object
	s.blockRemoves(c)
	s.assertDestroyOperationBlocked(c, true)

	// prevent all-changes
	s.blockAllChanges(c)
	s.assertDestroyOperationBlocked(c, true)
}

func (s *blocksSuite) TestBlockOperationErrorRemove(c *gc.C) {
	// prevent destroy-environment
	s.blockDestroys(c)
	s.assertRemoveOperationBlocked(c, false)

	// prevent remove-object
	s.blockRemoves(c)
	s.assertRemoveOperationBlocked(c, true)

	// prevent all-changes
	s.blockAllChanges(c)
	s.assertRemoveOperationBlocked(c, true)
}

func (s *blocksSuite) TestBlockOperationErrorChange(c *gc.C) {
	// prevent destroy-environment
	s.blockDestroys(c)
	s.assertChangeOperationBlocked(c, false)

	// prevent remove-object
	s.blockRemoves(c)
	s.assertChangeOperationBlocked(c, false)

	// prevent all-changes
	s.blockAllChanges(c)
	s.assertChangeOperationBlocked(c, true)
}

func (s *blocksSuite) blockDestroys(c *gc.C) {
	s.destroy, s.remove, s.change = true, false, false
}

func (s *blocksSuite) blockRemoves(c *gc.C) {
	s.remove, s.destroy, s.change = true, false, false
}

func (s *blocksSuite) blockAllChanges(c *gc.C) {
	s.change, s.destroy, s.remove = true, false, false
}

func (s *blocksSuite) assertDestroyOperationBlocked(c *gc.C, value bool) {
	s.assertOperationBlocked(c, common.DestroyOperation, value)
}

func (s *blocksSuite) assertRemoveOperationBlocked(c *gc.C, value bool) {
	s.assertOperationBlocked(c, common.RemoveOperation, value)
}

func (s *blocksSuite) assertChangeOperationBlocked(c *gc.C, value bool) {
	s.assertOperationBlocked(c, common.ChangeOperation, value)
}

func (s *blocksSuite) assertOperationBlocked(c *gc.C, operation common.Operation, value bool) {
	c.Assert(common.IsOperationBlocked(operation, s.getCurrentConfig(c)), gc.Equals, value)
}

func (s *blocksSuite) getCurrentConfig(c *gc.C) *config.Config {
	cfg, err := s.cfg.Apply(map[string]interface{}{
		"block-destroy-environment": s.destroy,
		"block-remove-object":       s.remove,
		"block-all-changes":         s.change,
	})
	c.Assert(err, jc.ErrorIsNil)
	return cfg
}

type blockCheckerSuite struct {
	blocksSuite
	getter       *mockGetter
	blockchecker *common.BlockChecker
}

var _ = gc.Suite(&blockCheckerSuite{})

func (s *blockCheckerSuite) SetUpTest(c *gc.C) {
	s.blocksSuite.SetUpTest(c)
	s.getter = &mockGetter{
		suite: s,
		c:     c,
	}
	s.blockchecker = common.NewBlockChecker(s.getter)
}

type mockGetter struct {
	suite *blockCheckerSuite
	c     *gc.C
}

func (mock *mockGetter) EnvironConfig() (*config.Config, error) {
	return mock.suite.getCurrentConfig(mock.c), nil
}

func (s *blockCheckerSuite) TestDestroyBlockChecker(c *gc.C) {
	s.blockDestroys(c)
	s.assertDestroyBlocked(c)

	s.blockRemoves(c)
	s.assertDestroyBlocked(c)

	s.blockAllChanges(c)
	s.assertDestroyBlocked(c)
}

func (s *blockCheckerSuite) TestRemoveBlockChecker(c *gc.C) {
	s.blockDestroys(c)
	s.assertRemoveBlocked(c, false)

	s.blockRemoves(c)
	s.assertRemoveBlocked(c, true)

	s.blockAllChanges(c)
	s.assertRemoveBlocked(c, true)
}

func (s *blockCheckerSuite) TestChangeBlockChecker(c *gc.C) {
	s.blockDestroys(c)
	s.assertChangeBlocked(c, false)

	s.blockRemoves(c)
	s.assertChangeBlocked(c, false)

	s.blockAllChanges(c)
	s.assertChangeBlocked(c, true)
}

func (s *blockCheckerSuite) assertDestroyBlocked(c *gc.C) {
	c.Assert(errors.Cause(s.blockchecker.DestroyAllowed()), gc.Equals, common.ErrOperationBlocked)
}

func (s *blockCheckerSuite) assertRemoveBlocked(c *gc.C, blocked bool) {
	if blocked {
		c.Assert(errors.Cause(s.blockchecker.RemoveAllowed()), gc.Equals, common.ErrOperationBlocked)
	} else {
		c.Assert(errors.Cause(s.blockchecker.RemoveAllowed()), jc.ErrorIsNil)
	}
}

func (s *blockCheckerSuite) assertChangeBlocked(c *gc.C, blocked bool) {
	if blocked {
		c.Assert(errors.Cause(s.blockchecker.ChangeAllowed()), gc.Equals, common.ErrOperationBlocked)
	} else {
		c.Assert(errors.Cause(s.blockchecker.ChangeAllowed()), jc.ErrorIsNil)
	}
}
