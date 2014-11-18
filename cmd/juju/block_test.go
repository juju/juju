// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"
	gc "gopkg.in/check.v1"
	"strings"

	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/testing"
)

type ProtectionCommandSuite struct {
	testing.FakeJujuHomeSuite
	mockClient *mockClient
}

func (s *ProtectionCommandSuite) assertErrorMatches(c *gc.C, err error, expected string) {
	c.Assert(
		err,
		gc.ErrorMatches,
		expected)
}

func (s *ProtectionCommandSuite) SetUpTest(c *gc.C) {
	s.FakeJujuHomeSuite.SetUpTest(c)
	s.mockClient = &mockClient{}
	s.PatchValue(&getBlockClientAPI, func(p *ProtectionCommand) (BlockClientAPI, error) {
		return s.mockClient, nil
	})
}

func (s *ProtectionCommandSuite) TearDownTest(c *gc.C) {
	s.FakeJujuHomeSuite.TearDownTest(c)
}

type mockClient struct {
	operation        string
	expectedVarValue bool
}

func (c *mockClient) Close() error {
	return nil
}

func (c *mockClient) EnvironmentSet(attrs map[string]interface{}) error {
	expectedOp := config.BlockKeyPrefix + strings.ToLower(c.operation)
	if val := attrs[expectedOp]; val == "" || val != c.expectedVarValue {
		return fmt.Errorf("env var %q was %v but was expected %v", expectedOp, val, c.expectedVarValue)
	}
	return nil
}

type BlockCommandSuite struct {
	ProtectionCommandSuite
}

var _ = gc.Suite(&BlockCommandSuite{})

func runBlockCommand(c *gc.C, args ...string) error {
	_, err := testing.RunCommand(c, envcmd.Wrap(&BlockCommand{}), args...)
	return err
}

func (s *BlockCommandSuite) runBlockTestAndCompare(c *gc.C, operation string, expectedVarValue bool) {
	s.mockClient.operation = operation
	s.mockClient.expectedVarValue = expectedVarValue
	err := runBlockCommand(c, operation)
	c.Assert(err, gc.IsNil)
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
	s.runBlockTestAndCompare(c, "DESTROY-ENVIRONMENT", true)
}

func (s *BlockCommandSuite) TestBlockCmdValidDestroyEnvOperation(c *gc.C) {
	s.runBlockTestAndCompare(c, "destroy-environment", true)
}
