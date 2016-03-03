// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package featuretests

import (
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/commands"
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
}

func (s *cmdEnvironmentSuite) run(c *gc.C, args ...string) *cmd.Context {
	context := testing.Context(c)
	jujuCmd := commands.NewJujuCommand(context)
	err := testing.InitCommand(jujuCmd, args)
	c.Assert(err, jc.ErrorIsNil)
	err = jujuCmd.Run(context)
	c.Assert(err, jc.ErrorIsNil)
	return context
}

func (s *cmdEnvironmentSuite) TestEnvironmentShareCmdStack(c *gc.C) {
	username := "bar@ubuntuone"
	context := s.run(c, "share-model", username)
	obtained := strings.Replace(testing.Stdout(context), "\n", "", -1)
	expected := ""
	c.Assert(obtained, gc.Equals, expected)

	user := names.NewUserTag(username)
	modelUser, err := s.State.ModelUser(user)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(modelUser.UserName(), gc.Equals, user.Canonical())
	c.Assert(modelUser.CreatedBy(), gc.Equals, s.AdminUserTag(c).Canonical())
	lastConn, err := modelUser.LastConnection()
	c.Assert(err, jc.Satisfies, state.IsNeverConnectedError)
	c.Assert(lastConn.IsZero(), jc.IsTrue)
}

func (s *cmdEnvironmentSuite) TestEnvironmentUnshareCmdStack(c *gc.C) {
	// Firstly share an environment with a user
	username := "bar@ubuntuone"
	context := s.run(c, "share-model", username)
	user := names.NewUserTag(username)
	modelUser, err := s.State.ModelUser(user)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(modelUser, gc.NotNil)

	// Because we are calling into juju through the main command,
	// and the main command adds a warning logging writer, we need
	// to clear the logging writers here.
	loggo.RemoveWriter("warning")

	// Then test that the unshare command stack is hooked up
	context = s.run(c, "unshare-model", username)
	obtained := strings.Replace(testing.Stdout(context), "\n", "", -1)
	expected := ""
	c.Assert(obtained, gc.Equals, expected)

	modelUser, err = s.State.ModelUser(user)
	c.Assert(errors.IsNotFound(err), jc.IsTrue)
	c.Assert(modelUser, gc.IsNil)
}

func (s *cmdEnvironmentSuite) TestEnvironmentUsersCmd(c *gc.C) {
	// Firstly share an environment with a user
	username := "bar@ubuntuone"
	context := s.run(c, "share-model", username)
	user := names.NewUserTag(username)
	modelUser, err := s.State.ModelUser(user)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(modelUser, gc.NotNil)

	// Because we are calling into juju through the main command,
	// and the main command adds a warning logging writer, we need
	// to clear the logging writers here.
	loggo.RemoveWriter("warning")

	context = s.run(c, "list-shares")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(testing.Stdout(context), gc.Equals, ""+
		"NAME           DATE CREATED  LAST CONNECTION\n"+
		"admin@local    just now      just now\n"+
		"bar@ubuntuone  just now      never connected\n"+
		"\n")

}

func (s *cmdEnvironmentSuite) TestGet(c *gc.C) {
	err := s.State.UpdateModelConfig(map[string]interface{}{"special": "known"}, nil, nil)
	c.Assert(err, jc.ErrorIsNil)

	context := s.run(c, "get-model-config", "special")
	c.Assert(testing.Stdout(context), gc.Equals, "known\n")
}

func (s *cmdEnvironmentSuite) TestSet(c *gc.C) {
	s.run(c, "set-model-config", "special=known")
	s.assertEnvValue(c, "special", "known")
}

func (s *cmdEnvironmentSuite) TestUnset(c *gc.C) {
	err := s.State.UpdateModelConfig(map[string]interface{}{"special": "known"}, nil, nil)
	c.Assert(err, jc.ErrorIsNil)

	s.run(c, "unset-model-config", "special")
	s.assertEnvValueMissing(c, "special")
}

func (s *cmdEnvironmentSuite) TestRetryProvisioning(c *gc.C) {
	s.Factory.MakeMachine(c, &factory.MachineParams{
		Jobs: []state.MachineJob{state.JobManageModel},
	})
	ctx := s.run(c, "retry-provisioning", "0")
	output := testing.Stderr(ctx)
	stripped := strings.Replace(output, "\n", "", -1)
	c.Check(stripped, gc.Equals, `machine 0 is not in an error state`)
}

func (s *cmdEnvironmentSuite) assertEnvValue(c *gc.C, key string, expected interface{}) {
	envConfig, err := s.State.ModelConfig()
	c.Assert(err, jc.ErrorIsNil)
	value, found := envConfig.AllAttrs()[key]
	c.Assert(found, jc.IsTrue)
	c.Assert(value, gc.Equals, expected)
}

func (s *cmdEnvironmentSuite) assertEnvValueMissing(c *gc.C, key string) {
	envConfig, err := s.State.ModelConfig()
	c.Assert(err, jc.ErrorIsNil)
	_, found := envConfig.AllAttrs()[key]
	c.Assert(found, jc.IsFalse)
}
