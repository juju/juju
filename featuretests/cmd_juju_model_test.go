// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package featuretests

import (
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"
	"gopkg.in/yaml.v1"

	"github.com/juju/juju/cmd/juju/commands"
	"github.com/juju/juju/core/description"
	"github.com/juju/juju/feature"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/permission"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
)

type cmdModelSuite struct {
	jujutesting.RepoSuite
}

func (s *cmdModelSuite) SetUpTest(c *gc.C) {
	s.RepoSuite.SetUpTest(c)
}

func (s *cmdModelSuite) run(c *gc.C, args ...string) *cmd.Context {
	context := testing.Context(c)
	jujuCmd := commands.NewJujuCommand(context)
	err := testing.InitCommand(jujuCmd, args)
	c.Assert(err, jc.ErrorIsNil)
	err = jujuCmd.Run(context)
	c.Assert(err, jc.ErrorIsNil)
	return context
}

func (s *cmdModelSuite) TestGrantModelCmdStack(c *gc.C) {
	username := "bar@ubuntuone"
	context := s.run(c, "grant", username, "read", "controller")
	obtained := strings.Replace(testing.Stdout(context), "\n", "", -1)
	expected := ""
	c.Assert(obtained, gc.Equals, expected)

	user := names.NewUserTag(username)
	modelUser, err := s.State.UserAccess(user, s.State.ModelTag())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(modelUser.UserName, gc.Equals, user.Canonical())
	c.Assert(modelUser.CreatedBy.Canonical(), gc.Equals, s.AdminUserTag(c).Canonical())
	lastConn, err := s.State.LastModelConnection(modelUser.UserTag)
	c.Assert(err, jc.Satisfies, state.IsNeverConnectedError)
	c.Assert(lastConn.IsZero(), jc.IsTrue)
}

func (s *cmdModelSuite) TestRevokeModelCmdStack(c *gc.C) {
	// Firstly share a model with a user
	username := "bar@ubuntuone"
	s.Factory.MakeModelUser(c, &factory.ModelUserParams{
		User: username, Access: permission.ReadAccess})

	// Because we are calling into juju through the main command,
	// and the main command adds a warning logging writer, we need
	// to clear the logging writers here.
	loggo.RemoveWriter("warning")

	// Then test that the unshare command stack is hooked up
	context := s.run(c, "revoke", username, "read", "controller")
	obtained := strings.Replace(testing.Stdout(context), "\n", "", -1)
	expected := ""
	c.Assert(obtained, gc.Equals, expected)

	user := names.NewUserTag(username)
	modelUser, err := s.State.UserAccess(user, s.State.ModelTag())
	c.Assert(errors.IsNotFound(err), jc.IsTrue)
	c.Assert(modelUser, gc.DeepEquals, permission.UserAccess{})
}

func (s *cmdModelSuite) TestModelUsersCmd(c *gc.C) {
	// Firstly share an model with a user
	username := "bar@ubuntuone"
	context := s.run(c, "grant", username, "read", "controller")
	user := names.NewUserTag(username)
	modelUser, err := s.State.UserAccess(user, s.State.ModelTag())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(modelUser, gc.NotNil)

	// Because we are calling into juju through the main command,
	// and the main command adds a warning logging writer, we need
	// to clear the logging writers here.
	loggo.RemoveWriter("warning")

	context = s.run(c, "list-users", "controller")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(testing.Stdout(context), gc.Equals, ""+
		"NAME           DISPLAY NAME  ACCESS  LAST CONNECTION\n"+
		"admin@local*   admin         admin   just now\n"+
		"bar@ubuntuone                read    never connected\n"+
		"\n")

}

func (s *cmdModelSuite) TestGet(c *gc.C) {
	err := s.State.UpdateModelConfig(map[string]interface{}{"special": "known"}, nil, nil)
	c.Assert(err, jc.ErrorIsNil)

	context := s.run(c, "model-config", "special")
	c.Assert(testing.Stdout(context), gc.Equals, "known\n")
}

func (s *cmdModelSuite) TestSet(c *gc.C) {
	s.run(c, "model-config", "special=known")
	s.assertEnvValue(c, "special", "known")
}

func (s *cmdModelSuite) TestUnset(c *gc.C) {
	err := s.State.UpdateModelConfig(map[string]interface{}{"special": "known"}, nil, nil)
	c.Assert(err, jc.ErrorIsNil)

	s.run(c, "model-config", "--reset", "special")
	s.assertEnvValueMissing(c, "special")
}

func (s *cmdModelSuite) TestRetryProvisioning(c *gc.C) {
	s.Factory.MakeMachine(c, &factory.MachineParams{
		Jobs: []state.MachineJob{state.JobManageModel},
	})
	ctx := s.run(c, "retry-provisioning", "0")
	output := testing.Stderr(ctx)
	stripped := strings.Replace(output, "\n", "", -1)
	c.Check(stripped, gc.Equals, `machine 0 is not in an error state`)
}

func (s *cmdModelSuite) TestDumpModel(c *gc.C) {
	s.SetFeatureFlags(feature.DeveloperMode)
	s.Factory.MakeMachine(c, &factory.MachineParams{
		Jobs: []state.MachineJob{state.JobManageModel},
	})
	ctx := s.run(c, "dump-model")
	output := testing.Stdout(ctx)
	// The output is yaml formatted output that is a model description.
	model, err := description.Deserialize([]byte(output))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(model.Config()["name"], gc.Equals, "controller")
}

func (s *cmdModelSuite) TestDumpModelDB(c *gc.C) {
	s.SetFeatureFlags(feature.DeveloperMode)
	s.Factory.MakeMachine(c, &factory.MachineParams{
		Jobs: []state.MachineJob{state.JobManageModel},
	})
	ctx := s.run(c, "dump-db")
	output := testing.Stdout(ctx)
	// The output is map of collection names to documents.
	// Defaults to yaml output.
	var valueMap map[string]interface{}
	err := yaml.Unmarshal([]byte(output), &valueMap)
	c.Assert(err, jc.ErrorIsNil)
	c.Logf("%#v", valueMap)
	model := valueMap["models"]
	// yaml unmarshals maps with interface keys.
	modelMap, ok := model.(map[interface{}]interface{})
	c.Assert(ok, jc.IsTrue)
	c.Assert(modelMap["name"], gc.Equals, "controller")
}

func (s *cmdModelSuite) assertEnvValue(c *gc.C, key string, expected interface{}) {
	envConfig, err := s.State.ModelConfig()
	c.Assert(err, jc.ErrorIsNil)
	value, found := envConfig.AllAttrs()[key]
	c.Assert(found, jc.IsTrue)
	c.Assert(value, gc.Equals, expected)
}

func (s *cmdModelSuite) assertEnvValueMissing(c *gc.C, key string) {
	envConfig, err := s.State.ModelConfig()
	c.Assert(err, jc.ErrorIsNil)
	_, found := envConfig.AllAttrs()[key]
	c.Assert(found, jc.IsFalse)
}
