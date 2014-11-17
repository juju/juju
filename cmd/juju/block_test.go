// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	gc "gopkg.in/check.v1"

	jc "github.com/juju/testing/checkers"

	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/environs/config"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing"
	"strings"
)

type ProtectionCommandSuite struct {
	jujutesting.RepoSuite
}

func (s *ProtectionCommandSuite) assertEnvVariableSet(c *gc.C, operation string, expectedVarValue gc.Checker) {
	stateConfig, cfgErr := s.State.EnvironConfig()
	c.Assert(cfgErr, gc.IsNil)
	c.Assert(stateConfig.AllAttrs()[config.BlockKeyPrefix+strings.ToLower(operation)], expectedVarValue)
}

func (s *ProtectionCommandSuite) assertErrorMatches(c *gc.C, err error, expected string) {
	c.Assert(
		err,
		gc.ErrorMatches,
		expected)
}

type BlockCommandSuite struct {
	ProtectionCommandSuite
}

var _ = gc.Suite(&BlockCommandSuite{})

func runBlockCommand(c *gc.C, args ...string) error {
	_, err := testing.RunCommand(c, envcmd.Wrap(&BlockCommand{}), args...)
	return err
}

func (s *BlockCommandSuite) runBlockTestAndAssert(c *gc.C, operation string, expectedVarValue gc.Checker) {
	err := runBlockCommand(c, operation)
	c.Assert(err, gc.IsNil)
	s.assertEnvVariableSet(c, operation, expectedVarValue)
}

func (s *BlockCommandSuite) TestBlockCmdNoOperation(c *gc.C) {
	s.assertErrorMatches(c, runBlockCommand(c), `.*specify operation.*`)
}

func (s *BlockCommandSuite) TestBlockCmdMoreThanOneOperation(c *gc.C) {
	s.assertErrorMatches(c, runBlockCommand(c, "destroy-environment", "change"), `.*specify operation.*`)
}

func (s *BlockCommandSuite) TestBlockCmdOperationWithSeparator(c *gc.C) {
	s.assertErrorMatches(c, runBlockCommand(c, "destroy-environment|"), `.*valid argument.*`)
}

func (s *BlockCommandSuite) TestBlockCmdUnknownJujuOperation(c *gc.C) {
	s.assertErrorMatches(c, runBlockCommand(c, "add-machine"), `.*valid argument.*`)
}

func (s *BlockCommandSuite) TestBlockCmdUnknownOperation(c *gc.C) {
	s.assertErrorMatches(c, runBlockCommand(c, "blah"), `.*valid argument.*`)
}

func (s *BlockCommandSuite) TestBlockCmdValidDestroyEnvOperationUpperCase(c *gc.C) {
	s.runBlockTestAndAssert(c, "DESTROY-ENVIRONMENT", jc.IsTrue)
}

func (s *BlockCommandSuite) TestBlockCmdValidDestroyEnvOperation(c *gc.C) {
	s.runBlockTestAndAssert(c, "destroy-environment", jc.IsTrue)
}

func (s *BlockCommandSuite) TestBlockCmdEnvironmentArg(c *gc.C) {
	_, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, gc.IsNil)
	err = runBlockCommand(c, "destroy-environment", "-e", "dummyenv")
	c.Assert(err, gc.IsNil)
}
