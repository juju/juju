// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environment_test

import (
	"github.com/juju/cmd"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/cmd/juju/environment"
	"github.com/juju/juju/testing"
)

type UnsetSuite struct {
	fakeEnvSuite
}

var _ = gc.Suite(&UnsetSuite{})

func (s *UnsetSuite) run(c *gc.C, args ...string) (*cmd.Context, error) {
	command := environment.NewUnsetCommand(s.fake)
	return testing.RunCommand(c, envcmd.Wrap(command), args...)
}

func (s *UnsetSuite) TestInit(c *gc.C) {
	unsetCmd := &environment.UnsetCommand{}
	// Only empty is a problem.
	err := testing.InitCommand(unsetCmd, []string{})
	c.Assert(err, gc.ErrorMatches, "no keys specified")
	// Everything else is fine.
	err = testing.InitCommand(unsetCmd, []string{"something", "weird"})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *UnsetSuite) TestPassesValues(c *gc.C) {
	_, err := s.run(c, "special", "running")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.fake.keys, jc.DeepEquals, []string{"special", "running"})
}

func (s *UnsetSuite) TestUnsettingKnownValue(c *gc.C) {
	_, err := s.run(c, "unknown")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.fake.keys, jc.DeepEquals, []string{"unknown"})
	// Command succeeds, but warning logged.
	expected := `key "unknown" is not defined in the current environment configuration: possible misspelling`
	c.Check(c.GetTestLog(), jc.Contains, expected)
}

func (s *UnsetSuite) TestBlockedError(c *gc.C) {
	s.fake.err = common.ErrOperationBlocked("TestBlockedError")
	_, err := s.run(c, "special")
	c.Assert(err, gc.Equals, cmd.ErrSilent)
	// msg is logged
	c.Check(c.GetTestLog(), jc.Contains, "TestBlockedError")
}
