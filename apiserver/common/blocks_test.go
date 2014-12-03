// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

type blocksSuite struct {
	testing.JujuConnSuite
}

func (s *blocksSuite) getTestCfg(c *gc.C) *config.Config {
	cfg, err := s.BackingState.EnvironConfig()
	c.Assert(err, jc.ErrorIsNil)
	return cfg
}

var _ = gc.Suite(&blocksSuite{})

func (s *blocksSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
}

func (s *blocksSuite) TestBlockllOperations(c *gc.C) {
	c.Assert(common.IsOperationBlocked(common.DestroyOperation, s.getTestCfg(c)), jc.IsFalse)
	c.Assert(common.IsOperationBlocked(common.RemoveOperation, s.getTestCfg(c)), jc.IsFalse)
	c.Assert(common.IsOperationBlocked(common.ChangeOperation, s.getTestCfg(c)), jc.IsFalse)
}

func (s *blocksSuite) TestBlockOperationErrorDestroy(c *gc.C) {
	//prevent destroy-environment
	s.AssertConfigParameterUpdated(c, "block-destroy-environment", true)
	c.Assert(common.IsOperationBlocked(common.DestroyOperation, s.getTestCfg(c)), jc.IsTrue)

	//prevent remove-object
	s.AssertConfigParameterUpdated(c, "block-destroy-environment", false)
	s.AssertConfigParameterUpdated(c, "block-remove-object", true)
	c.Assert(common.IsOperationBlocked(common.DestroyOperation, s.getTestCfg(c)), jc.IsTrue)

	//prevent all-changes
	s.AssertConfigParameterUpdated(c, "block-destroy-environment", false)
	s.AssertConfigParameterUpdated(c, "block-remove-object", false)
	s.AssertConfigParameterUpdated(c, "block-all-changes", true)
	c.Assert(common.IsOperationBlocked(common.DestroyOperation, s.getTestCfg(c)), jc.IsTrue)
}

func (s *blocksSuite) TestBlockOperationErrorRemove(c *gc.C) {
	//prevent destroy-environment
	s.AssertConfigParameterUpdated(c, "block-destroy-environment", true)
	c.Assert(common.IsOperationBlocked(common.RemoveOperation, s.getTestCfg(c)), jc.IsFalse)

	//prevent remove-object
	s.AssertConfigParameterUpdated(c, "block-destroy-environment", false)
	s.AssertConfigParameterUpdated(c, "block-remove-object", true)
	c.Assert(common.IsOperationBlocked(common.RemoveOperation, s.getTestCfg(c)), jc.IsTrue)

	//prevent all-changes
	s.AssertConfigParameterUpdated(c, "block-destroy-environment", false)
	s.AssertConfigParameterUpdated(c, "block-remove-object", false)
	s.AssertConfigParameterUpdated(c, "block-all-changes", true)
	c.Assert(common.IsOperationBlocked(common.RemoveOperation, s.getTestCfg(c)), jc.IsTrue)
}

func (s *blocksSuite) TestBlockOperationErrorChange(c *gc.C) {
	//prevent destroy-environment
	s.AssertConfigParameterUpdated(c, "block-destroy-environment", true)
	c.Assert(common.IsOperationBlocked(common.ChangeOperation, s.getTestCfg(c)), jc.IsFalse)

	//prevent remove-object
	s.AssertConfigParameterUpdated(c, "block-destroy-environment", false)
	s.AssertConfigParameterUpdated(c, "block-remove-object", true)
	c.Assert(common.IsOperationBlocked(common.ChangeOperation, s.getTestCfg(c)), jc.IsFalse)

	//prevent all-changes
	s.AssertConfigParameterUpdated(c, "block-destroy-environment", false)
	s.AssertConfigParameterUpdated(c, "block-remove-object", false)
	s.AssertConfigParameterUpdated(c, "block-all-changes", true)
	c.Assert(common.IsOperationBlocked(common.ChangeOperation, s.getTestCfg(c)), jc.IsTrue)
}
