// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package config

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/agent"
	coretesting "github.com/juju/juju/internal/testing"
)

type agentConfSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&agentConfSuite{})

func (s *agentConfSuite) TestChangeConfigSuccess(c *gc.C) {
	mcsw := &mockConfigSetterWriter{}
	conf := NewAgentConfigWithConfigSetterWriter(c.MkDir(), mcsw)
	err := conf.ChangeConfig(func(agent.ConfigSetter) error {
		return nil
	})

	c.Assert(err, jc.ErrorIsNil)
	c.Assert(mcsw.WriteCalled, jc.IsTrue)
}

func (s *agentConfSuite) TestChangeConfigMutateFailure(c *gc.C) {
	mcsw := &mockConfigSetterWriter{}
	conf := NewAgentConfigWithConfigSetterWriter(c.MkDir(), mcsw)

	err := conf.ChangeConfig(func(agent.ConfigSetter) error {
		return errors.New("blam")
	})

	c.Assert(err, gc.ErrorMatches, "blam")
	c.Assert(mcsw.WriteCalled, jc.IsFalse)
}

func (s *agentConfSuite) TestChangeConfigWriteFailure(c *gc.C) {
	mcsw := &mockConfigSetterWriter{
		WriteError: errors.New("boom"),
	}
	conf := NewAgentConfigWithConfigSetterWriter(c.MkDir(), mcsw)
	err := conf.ChangeConfig(func(agent.ConfigSetter) error {
		return nil
	})

	c.Assert(err, gc.ErrorMatches, "cannot write agent configuration: boom")
	c.Assert(mcsw.WriteCalled, jc.IsTrue)
}

type mockConfigSetterWriter struct {
	agent.ConfigSetterWriter
	WriteError  error
	WriteCalled bool
}

func (c *mockConfigSetterWriter) Write() error {
	c.WriteCalled = true
	return c.WriteError
}
