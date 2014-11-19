// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"strings"

	gc "gopkg.in/check.v1"

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

type mockClient struct {
	cfg map[string]interface{}
}

func (c *mockClient) Close() error {
	return nil
}

func (c *mockClient) EnvironmentSet(attrs map[string]interface{}) error {
	c.cfg = attrs
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

func (s *BlockCommandSuite) runBlockTestAndCompare(c *gc.C, operation string, expectedValue bool) {
	err := runBlockCommand(c, operation)
	c.Assert(err, gc.IsNil)

	expectedOp := config.BlockKeyPrefix + strings.ToLower(operation)
	c.Assert(s.mockClient.cfg[expectedOp], gc.Equals, expectedValue)
}

func (s *BlockCommandSuite) TestBlockCmdNoOperation(c *gc.C) {
	s.assertErrorMatches(c, runBlockCommand(c), `.*must specify one of.*`)
}

func (s *BlockCommandSuite) TestBlockCmdMoreThanOneOperation(c *gc.C) {
	s.assertErrorMatches(c, runBlockCommand(c, "destroy-environment", "change"), `.*must specify one of.*`)
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
