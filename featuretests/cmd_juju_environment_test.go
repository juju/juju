// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package featuretests

import (
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	cmdenvironment "github.com/juju/juju/cmd/juju/environment"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/feature"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
)

type cmdEnvironmentSuite struct {
	jujutesting.RepoSuite
}

func (s *cmdEnvironmentSuite) SetUpTest(c *gc.C) {
	s.RepoSuite.SetUpTest(c)
	s.SetFeatureFlags(feature.JES)
}

func (s *cmdEnvironmentSuite) run(c *gc.C, args ...string) *cmd.Context {
	command := cmdenvironment.NewSuperCommand()
	context, err := testing.RunCommand(c, command, args...)
	c.Assert(err, jc.ErrorIsNil)
	return context
}

func (s *cmdEnvironmentSuite) TestEnvironmentShareCmdStack(c *gc.C) {
	username := "bar@ubuntuone"
	context := s.run(c, "share", username)
	obtained := strings.Replace(testing.Stdout(context), "\n", "", -1)
	expected := ""
	c.Assert(obtained, gc.Equals, expected)

	user := names.NewUserTag(username)
	envUser, err := s.State.EnvironmentUser(user)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(envUser.UserName(), gc.Equals, user.Username())
	c.Assert(envUser.CreatedBy(), gc.Equals, s.AdminUserTag(c).Username())
	lastConn, err := envUser.LastConnection()
	c.Assert(err, jc.Satisfies, state.IsNeverConnectedError)
	c.Assert(lastConn.IsZero(), jc.IsTrue)
}

func (s *cmdEnvironmentSuite) TestEnvironmentUnshareCmdStack(c *gc.C) {
	// Firstly share an environment with a user
	username := "bar@ubuntuone"
	context := s.run(c, "share", username)
	user := names.NewUserTag(username)
	envuser, err := s.State.EnvironmentUser(user)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(envuser, gc.NotNil)

	// Then test that the unshare command stack is hooked up
	context = s.run(c, "unshare", username)
	obtained := strings.Replace(testing.Stdout(context), "\n", "", -1)
	expected := ""
	c.Assert(obtained, gc.Equals, expected)

	envuser, err = s.State.EnvironmentUser(user)
	c.Assert(errors.IsNotFound(err), jc.IsTrue)
	c.Assert(envuser, gc.IsNil)
}

func (s *cmdEnvironmentSuite) TestEnvironmentUsersCmd(c *gc.C) {
	// Firstly share an environment with a user
	username := "bar@ubuntuone"
	context := s.run(c, "share", username)
	user := names.NewUserTag(username)
	envuser, err := s.State.EnvironmentUser(user)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(envuser, gc.NotNil)

	context = s.run(c, "users")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(testing.Stdout(context), gc.Equals, ""+
		"NAME               DATE CREATED  LAST CONNECTION\n"+
		"dummy-admin@local  just now      just now\n"+
		"bar@ubuntuone      just now      never connected\n"+
		"\n")

}

func (s *cmdEnvironmentSuite) TestGet(c *gc.C) {
	err := s.State.UpdateEnvironConfig(map[string]interface{}{"special": "known"}, nil, nil)
	c.Assert(err, jc.ErrorIsNil)

	context := s.run(c, "get", "special")
	c.Assert(testing.Stdout(context), gc.Equals, "known\n")
}

func (s *cmdEnvironmentSuite) TestSet(c *gc.C) {
	s.run(c, "set", "special=known")
	s.assertEnvValue(c, "special", "known")
}

func (s *cmdEnvironmentSuite) TestUnset(c *gc.C) {
	err := s.State.UpdateEnvironConfig(map[string]interface{}{"special": "known"}, nil, nil)
	c.Assert(err, jc.ErrorIsNil)

	s.run(c, "unset", "special")
	s.assertEnvValueMissing(c, "special")
}

func (s *cmdEnvironmentSuite) TestRetryProvisioning(c *gc.C) {
	s.Factory.MakeMachine(c, &factory.MachineParams{
		Jobs: []state.MachineJob{state.JobManageEnviron},
	})
	ctx := s.run(c, "retry-provisioning", "0")
	output := testing.Stderr(ctx)
	stripped := strings.Replace(output, "\n", "", -1)
	c.Check(stripped, gc.Equals, `machine 0 is not in an error state`)
}

func (s *cmdEnvironmentSuite) TestGetConstraints(c *gc.C) {
	cons := constraints.Value{CpuPower: uint64p(250)}
	err := s.State.SetEnvironConstraints(cons)
	c.Assert(err, jc.ErrorIsNil)

	ctx := s.run(c, "get-constraints")
	c.Check(testing.Stdout(ctx), gc.Equals, "cpu-power=250\n")
}

func (s *cmdEnvironmentSuite) TestSetConstraints(c *gc.C) {
	s.run(c, "set-constraints", "mem=4G", "cpu-power=250")

	cons, err := s.State.EnvironConstraints()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cons, gc.DeepEquals, constraints.Value{
		CpuPower: uint64p(250),
		Mem:      uint64p(4096),
	})
}

func (s *cmdEnvironmentSuite) assertEnvValue(c *gc.C, key string, expected interface{}) {
	envConfig, err := s.State.EnvironConfig()
	c.Assert(err, jc.ErrorIsNil)
	value, found := envConfig.AllAttrs()[key]
	c.Assert(found, jc.IsTrue)
	c.Assert(value, gc.Equals, expected)
}

func (s *cmdEnvironmentSuite) assertEnvValueMissing(c *gc.C, key string) {
	envConfig, err := s.State.EnvironConfig()
	c.Assert(err, jc.ErrorIsNil)
	_, found := envConfig.AllAttrs()[key]
	c.Assert(found, jc.IsFalse)
}
