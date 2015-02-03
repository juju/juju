// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package block_test

import (
	stdtesting "testing"

	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/block"
	"github.com/juju/juju/testing"
)

func TestAll(t *stdtesting.T) {
	testing.MgoTestPackage(t)
}

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
	s.PatchValue(block.ClientGetter, func(p *block.ProtectionCommand) (block.ClientAPI, error) {
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
