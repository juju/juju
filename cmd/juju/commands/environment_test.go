// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"strings"

	"github.com/juju/cmd"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/constraints"
	"github.com/juju/juju/feature"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
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

func (s *EnvironmentSuite) TestRetryProvisioning(c *gc.C) {
	s.Factory.MakeMachine(c, &factory.MachineParams{
		Jobs: []state.MachineJob{state.JobManageEnviron},
	})
	ctx, err := s.RunEnvironmentCommand(c, "retry-provisioning", "0")
	c.Assert(err, jc.ErrorIsNil)

	output := testing.Stderr(ctx)
	stripped := strings.Replace(output, "\n", "", -1)
	c.Check(stripped, gc.Equals, `machine 0 is not in an error state`)
}

func (s *EnvironmentSuite) TestCreate(c *gc.C) {
	s.SetFeatureFlags(feature.JES)
	// The JujuConnSuite doesn't set up an ssh key in the fake home dir,
	// so fake one on the command line.  The dummy provider also expects
	// a config value for 'state-server'.
	context, err := s.RunEnvironmentCommand(c, "create", "new-env", "authorized-keys=fake-key", "state-server=false")
	c.Check(err, jc.ErrorIsNil)
	c.Check(testing.Stdout(context), gc.Equals, "")
	c.Check(testing.Stderr(context), gc.Equals, "")
}

func uint64p(val uint64) *uint64 {
	return &val
}

func (s *EnvironmentSuite) TestGetConstraints(c *gc.C) {
	cons := constraints.Value{CpuPower: uint64p(250)}
	err := s.State.SetEnvironConstraints(cons)
	c.Assert(err, jc.ErrorIsNil)

	ctx, err := s.RunEnvironmentCommand(c, "get-constraints")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(testing.Stdout(ctx), gc.Equals, "cpu-power=250\n")
}

func (s *EnvironmentSuite) TestSetConstraints(c *gc.C) {
	_, err := s.RunEnvironmentCommand(c, "set-constraints", "mem=4G", "cpu-power=250")
	c.Assert(err, jc.ErrorIsNil)

	cons, err := s.State.EnvironConstraints()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cons, gc.DeepEquals, constraints.Value{
		CpuPower: uint64p(250),
		Mem:      uint64p(4096),
	})
}
