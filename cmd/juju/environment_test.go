// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"github.com/juju/cmd"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/testing"
)

// EnvironmentSuite tests the connectivity of all the environment subcommands.
// These tests go from the command line, api client, api server, db. The db
// changes are then checked.  Only one test for each command is done here to
// check connectivity.  Exhaustive unit tests are at each layer.
type EnvironmentSuite struct {
	jujutesting.JujuConnSuite
}

var _ = gc.Suite(&EnvironmentSuite{})

func (s *EnvironmentSuite) assertEnvValue(c *gc.C, key string, expected interface{}) {
	envConfig, err := s.State.EnvironConfig()
	c.Assert(err, jc.ErrorIsNil)
	value, found := envConfig.AllAttrs()[key]
	c.Assert(found, jc.IsTrue)
	c.Assert(value, gc.Equals, expected)
}

func (s *EnvironmentSuite) assertEnvValueMissing(c *gc.C, key string) {
	envConfig, err := s.State.EnvironConfig()
	c.Assert(err, jc.ErrorIsNil)
	_, found := envConfig.AllAttrs()[key]
	c.Assert(found, jc.IsFalse)
}

func (s *EnvironmentSuite) RunEnvironmentCommand(c *gc.C, commands ...string) (*cmd.Context, error) {
	args := []string{"environment"}
	args = append(args, commands...)
	context := testing.Context(c)
	juju := NewJujuCommand(context)
	if err := testing.InitCommand(juju, args); err != nil {
		return context, err
	}
	return context, juju.Run(context)
}

func (s *EnvironmentSuite) TestGet(c *gc.C) {
	err := s.State.UpdateEnvironConfig(map[string]interface{}{"special": "known"}, nil, nil)
	c.Assert(err, jc.ErrorIsNil)

	context, err := s.RunEnvironmentCommand(c, "get", "special")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(testing.Stdout(context), gc.Equals, "known\n")
}

func (s *EnvironmentSuite) TestSet(c *gc.C) {
	_, err := s.RunEnvironmentCommand(c, "set", "special=known")
	c.Assert(err, jc.ErrorIsNil)
	s.assertEnvValue(c, "special", "known")
}

func (s *EnvironmentSuite) TestUnset(c *gc.C) {
	err := s.State.UpdateEnvironConfig(map[string]interface{}{"special": "known"}, nil, nil)
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.RunEnvironmentCommand(c, "unset", "special")
	c.Assert(err, jc.ErrorIsNil)
	s.assertEnvValueMissing(c, "special")
}
