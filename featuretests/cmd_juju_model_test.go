// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package featuretests

import (
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/cmd/cmdtesting"
	"github.com/juju/description/v2"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/yaml.v2"

	"github.com/juju/juju/cmd/juju/commands"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/feature"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing/factory"
)

type cmdModelSuite struct {
	jujutesting.RepoSuite
}

func (s *cmdModelSuite) SetUpTest(c *gc.C) {
	s.RepoSuite.SetUpTest(c)
}

func (s *cmdModelSuite) run(c *gc.C, args ...string) *cmd.Context {
	context := cmdtesting.Context(c)
	jujuCmd := commands.NewJujuCommand(context, "")
	err := cmdtesting.InitCommand(jujuCmd, args)
	c.Assert(err, jc.ErrorIsNil)
	err = jujuCmd.Run(context)
	c.Assert(err, jc.ErrorIsNil)
	return context
}

func (s *cmdModelSuite) TestGrantModelCmdStack(c *gc.C) {
	username := "bar@ubuntuone"
	context := s.run(c, "grant", username, "read", "controller")
	obtained := strings.Replace(cmdtesting.Stdout(context), "\n", "", -1)
	expected := ""
	c.Assert(obtained, gc.Equals, expected)

	user := names.NewUserTag(username)
	modelUser, err := s.State.UserAccess(user, s.Model.ModelTag())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(modelUser.UserName, gc.Equals, user.Id())
	c.Assert(modelUser.CreatedBy.Id(), gc.Equals, s.AdminUserTag(c).Id())
	lastConn, err := s.Model.LastModelConnection(modelUser.UserTag)
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
	obtained := strings.Replace(cmdtesting.Stdout(context), "\n", "", -1)
	expected := ""
	c.Assert(obtained, gc.Equals, expected)

	user := names.NewUserTag(username)
	modelUser, err := s.State.UserAccess(user, s.Model.ModelTag())
	c.Assert(errors.IsNotFound(err), jc.IsTrue)
	c.Assert(modelUser, gc.DeepEquals, permission.UserAccess{})
}

func (s *cmdModelSuite) TestModelUsersCmd(c *gc.C) {
	// Firstly share an model with a user
	username := "bar@ubuntuone"
	s.run(c, "grant", username, "read", "controller")
	user := names.NewUserTag(username)
	modelUser, err := s.State.UserAccess(user, s.Model.ModelTag())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(modelUser, gc.NotNil)

	// Because we are calling into juju through the main command,
	// and the main command adds a warning logging writer, we need
	// to clear the logging writers here.
	loggo.RemoveWriter("warning")

	context := s.run(c, "list-users", "controller")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(context), gc.Equals, ""+
		"Name           Display name  Access  Last connection\n"+
		"admin*         admin         admin   just now\n"+
		"bar@ubuntuone                read    never connected\n"+
		"\n")

}

func (s *cmdModelSuite) TestModelConfigGet(c *gc.C) {
	err := s.Model.UpdateModelConfig(map[string]interface{}{"special": "known"}, nil)
	c.Assert(err, jc.ErrorIsNil)

	context := s.run(c, "model-config", "special")
	c.Assert(cmdtesting.Stdout(context), gc.Equals, "known\n")
}

func (s *cmdModelSuite) TestModelConfigSet(c *gc.C) {
	s.run(c, "model-config", "special=known")
	s.assertModelValue(c, "special", "known")
}

func (s *cmdModelSuite) TestModelConfigReset(c *gc.C) {
	err := s.Model.UpdateModelConfig(map[string]interface{}{"special": "known"}, nil)
	c.Assert(err, jc.ErrorIsNil)

	s.run(c, "model-config", "--reset", "special")
	s.assertModelValueMissing(c, "special")
}

func (s *cmdModelSuite) TestModelDefaultsGet(c *gc.C) {
	err := s.State.UpdateModelConfigDefaultValues(map[string]interface{}{"special": "known"}, nil, nil)
	c.Assert(err, jc.ErrorIsNil)

	context := s.run(c, "model-defaults", "special")
	c.Assert(cmdtesting.Stdout(context), gc.Equals, `
Attribute  Default  Controller
special    -        known

`[1:])
}

func (s *cmdModelSuite) TestModelDefaultsGetCloud(c *gc.C) {
	err := s.State.UpdateModelConfigDefaultValues(map[string]interface{}{"special": "known"}, nil, &environs.CloudRegionSpec{Cloud: "dummy"})
	c.Assert(err, jc.ErrorIsNil)

	context := s.run(c, "model-defaults", "dummy", "special")
	c.Assert(cmdtesting.Stdout(context), gc.Equals, `
Attribute  Default  Controller
special    -        known

`[1:])
}

func (s *cmdModelSuite) TestModelDefaultsGetRegion(c *gc.C) {
	err := s.State.UpdateModelConfigDefaultValues(map[string]interface{}{"special": "known"}, nil, &environs.CloudRegionSpec{"dummy", "dummy-region"})
	c.Assert(err, jc.ErrorIsNil)

	context := s.run(c, "model-defaults", "dummy-region", "special")
	c.Assert(cmdtesting.Stdout(context), gc.Equals, `
Attribute       Default  Controller
special         -        -
  dummy-region  known    -

`[1:])
}

func (s *cmdModelSuite) TestModelDefaultsSet(c *gc.C) {
	s.run(c, "model-defaults", "special=known")
	defaults, err := s.State.ModelConfigDefaultValues(s.Model.CloudName())
	c.Assert(err, jc.ErrorIsNil)
	value, found := defaults["special"]
	c.Assert(found, jc.IsTrue)
	c.Assert(value.Controller, gc.Equals, "known")
}

func (s *cmdModelSuite) TestModelDefaultsSetCloud(c *gc.C) {
	s.run(c, "model-defaults", "dummy", "special=known")
	defaults, err := s.State.ModelConfigDefaultValues(s.Model.CloudName())
	c.Assert(err, jc.ErrorIsNil)
	value, found := defaults["special"]
	c.Assert(found, jc.IsTrue)
	c.Assert(value.Controller, gc.Equals, "known")
	c.Assert(value.Regions, gc.HasLen, 0)
}

func (s *cmdModelSuite) TestModelDefaultsSetRegion(c *gc.C) {
	s.run(c, "model-defaults", "dummy/dummy-region", "special=known")
	defaults, err := s.State.ModelConfigDefaultValues(s.Model.CloudName())
	c.Assert(err, jc.ErrorIsNil)
	value, found := defaults["special"]
	c.Assert(found, jc.IsTrue)
	c.Assert(value.Controller, gc.IsNil)
	c.Assert(value.Regions, jc.SameContents, []config.RegionDefaultValue{{"dummy-region", "known"}})
}

func (s *cmdModelSuite) TestModelDefaultsReset(c *gc.C) {
	err := s.State.UpdateModelConfigDefaultValues(map[string]interface{}{"special": "known"}, nil, nil)
	c.Assert(err, jc.ErrorIsNil)

	s.run(c, "model-defaults", "--reset", "special")
	defaults, err := s.State.ModelConfigDefaultValues(s.Model.CloudName())
	c.Assert(err, jc.ErrorIsNil)
	_, found := defaults["special"]
	c.Assert(found, jc.IsFalse)
}

func (s *cmdModelSuite) TestModelDefaultsResetCloud(c *gc.C) {
	err := s.State.UpdateModelConfigDefaultValues(map[string]interface{}{"special": "known"}, nil, &environs.CloudRegionSpec{Cloud: "dummy"})
	c.Assert(err, jc.ErrorIsNil)

	s.run(c, "model-defaults", "dummy", "--reset", "special")
	defaults, err := s.State.ModelConfigDefaultValues(s.Model.CloudName())
	c.Assert(err, jc.ErrorIsNil)
	_, found := defaults["special"]
	c.Assert(found, jc.IsFalse)
}

func (s *cmdModelSuite) TestModelDefaultsResetRegion(c *gc.C) {
	err := s.State.UpdateModelConfigDefaultValues(map[string]interface{}{"special": "known"}, nil, &environs.CloudRegionSpec{"dummy", "dummy-region"})
	c.Assert(err, jc.ErrorIsNil)

	s.run(c, "model-defaults", "dummy-region", "--reset", "special")
	defaults, err := s.State.ModelConfigDefaultValues(s.Model.CloudName())
	c.Assert(err, jc.ErrorIsNil)
	_, found := defaults["special"]
	c.Assert(found, jc.IsFalse)
}

func (s *cmdModelSuite) TestRetryProvisioning(c *gc.C) {
	s.Factory.MakeMachine(c, &factory.MachineParams{
		Jobs: []state.MachineJob{state.JobManageModel},
	})
	ctx := s.run(c, "retry-provisioning", "0")
	output := cmdtesting.Stderr(ctx)
	stripped := strings.Replace(output, "\n", "", -1)
	c.Check(stripped, gc.Equals, `machine 0 is not in an error state`)
}

func (s *cmdModelSuite) TestDumpModel(c *gc.C) {
	s.SetFeatureFlags(feature.DeveloperMode)
	s.Factory.MakeMachine(c, &factory.MachineParams{
		Jobs: []state.MachineJob{state.JobManageModel},
	})
	ctx := s.run(c, "dump-model")
	output := cmdtesting.Stdout(ctx)
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
	output := cmdtesting.Stdout(ctx)
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

func (s *cmdModelSuite) assertModelValue(c *gc.C, key string, expected interface{}) {
	modelConfig, err := s.Model.ModelConfig()
	c.Assert(err, jc.ErrorIsNil)
	value, found := modelConfig.AllAttrs()[key]
	c.Assert(found, jc.IsTrue)
	c.Assert(value, gc.Equals, expected)
}

func (s *cmdModelSuite) assertModelValueMissing(c *gc.C, key string) {
	modelConfig, err := s.Model.ModelConfig()
	c.Assert(err, jc.ErrorIsNil)
	_, found := modelConfig.AllAttrs()[key]
	c.Assert(found, jc.IsFalse)
}
