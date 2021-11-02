// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"fmt"

	"github.com/golang/mock/gomock"
	"github.com/juju/cmd/v3"
	"github.com/juju/cmd/v3/cmdtesting"
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version/v2"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/cmd/juju/commands/mocks"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/core/model"
	coreos "github.com/juju/juju/core/os"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/environs/tools"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/jujuclient/jujuclienttesting"
	coretesting "github.com/juju/juju/testing"
	coretools "github.com/juju/juju/tools"
	jujuversion "github.com/juju/juju/version"
)

type UpgradeIAASControllerSuite struct {
	UpgradeBaseSuite
}

func (s *UpgradeIAASControllerSuite) SetUpTest(c *gc.C) {
	s.UpgradeBaseSuite.SetUpTest(c)
	err := s.ControllerStore.UpdateModel(jujutesting.ControllerName, "admin/dummy-model", jujuclient.ModelDetails{
		ModelType: model.IAAS,
		ModelUUID: coretesting.ModelTag.Id(),
	})
	c.Assert(err, jc.ErrorIsNil)
	s.resources = common.NewResources()
	s.authoriser = apiservertesting.FakeAuthorizer{
		Tag: s.AdminUserTag(c),
	}
	s.CmdBlockHelper = coretesting.NewCmdBlockHelper(s.APIState)
	c.Assert(s.CmdBlockHelper, gc.NotNil)
	s.AddCleanup(func(*gc.C) { s.CmdBlockHelper.Close() })
}

var _ = gc.Suite(&UpgradeIAASControllerSuite{})

var upgradeIAASControllerPassthroughTests = []upgradeTest{{
	about:          "unwanted extra argument",
	currentVersion: "1.0.0-ubuntu-amd64",
	args:           []string{"foo"},
	expectInitErr:  "unrecognized args:.*",
}, {
	about:          "invalid --agent-version value",
	currentVersion: "1.0.0-ubuntu-amd64",
	args:           []string{"--agent-version", "invalid-version"},
	expectInitErr:  "invalid version .*",
}, {
	about:          "latest supported stable release",
	available:      []string{"2.1.0-ubuntu-amd64", "2.1.2-ubuntu-i386", "2.1.3-ubuntu-amd64", "2.1-dev1-ubuntu-amd64"},
	currentVersion: "2.0.0-ubuntu-amd64",
	agentVersion:   "2.0.0",
	expectVersion:  "2.1.3",
}, {
	about:          "latest supported stable, when client is dev, explicit upload",
	available:      []string{"2.1-dev1-ubuntu-amd64", "2.1.0-ubuntu-amd64", "2.3-dev0-ubuntu-amd64", "3.0.1-ubuntu-amd64"},
	currentVersion: "2.1-dev0-ubuntu-amd64",
	agentVersion:   "2.0.0",
	args:           []string{"--build-agent"},
	expectVersion:  "2.1-dev0.1",
}, {
	about:          "upload with explicit version",
	currentVersion: "2.2.0-ubuntu-amd64",
	agentVersion:   "2.0.0",
	args:           []string{"--build-agent", "--agent-version", "2.7.3"},
	expectVersion:  "2.7.3.1",
	expectUploaded: []string{"2.7.3.1-ubuntu-amd64"},
}}

func (s *UpgradeIAASControllerSuite) upgradeControllerCommand() cmd.Command {
	cmd := &upgradeControllerCommand{}
	cmd.SetClientStore(s.ControllerStore)
	return modelcmd.WrapController(cmd)
}

func (s *UpgradeIAASControllerSuite) TestUpgrade(c *gc.C) {
	// Run a subset of the upgrade-juju tests ensuring the controller
	// model is selected.
	c.Assert(s.Model.Name(), gc.Equals, "controller")
	err := s.ControllerStore.SetCurrentModel("kontroll", "")
	c.Assert(err, jc.ErrorIsNil)

	s.assertUpgradeTests(c, upgradeIAASControllerPassthroughTests, s.upgradeControllerCommand)
}

func (s *UpgradeIAASControllerSuite) TestUpgradeWithRealUpload(c *gc.C) {
	s.Reset(c)
	s.PatchValue(&jujuversion.Current, version.MustParse("1.99.99"))
	cmd := s.upgradeControllerCommand()
	_, err := cmdtesting.RunCommand(c, cmd, "--build-agent")
	c.Assert(err, jc.ErrorIsNil)
	vers := coretesting.CurrentVersion(c)
	vers.Build = 1
	s.checkToolsUploaded(c, vers, vers.Number)
}

func (s *UpgradeIAASControllerSuite) TestUpgradeCorrectController(c *gc.C) {
	badControllerName := "not-the-right-controller"
	badControllerSelected := errors.New("bad controller selected")
	upgradeCommand := func() cmd.Command {
		backingStore := s.ControllerStore
		store := jujuclienttesting.WrapClientStore(backingStore)
		store.ControllerByNameFunc = func(name string) (*jujuclient.ControllerDetails, error) {
			if name == badControllerName {
				return nil, badControllerSelected
			}
			return backingStore.ControllerByName(name)
		}
		store.CurrentControllerFunc = func() (string, error) {
			return badControllerName, nil
		}
		s.ControllerStore = store
		return s.upgradeControllerCommand()
	}

	tests := []upgradeTest{
		{
			about:          "latest supported stable release with specified controller",
			available:      []string{"2.1.0-ubuntu-amd64", "2.1.2-ubuntu-i386", "2.1.3-ubuntu-amd64", "2.1-dev1-ubuntu-amd64"},
			currentVersion: "2.0.0-ubuntu-amd64",
			agentVersion:   "2.0.0",
			expectVersion:  "2.1.3",
			args:           []string{"--controller", "kontroll"},
		},
		{
			about:          "latest supported stable release without specified controller",
			available:      []string{"2.1.0-ubuntu-amd64", "2.1.2-ubuntu-i386", "2.1.3-ubuntu-amd64", "2.1-dev1-ubuntu-amd64"},
			currentVersion: "2.0.0-ubuntu-amd64",
			agentVersion:   "2.0.0",
			expectVersion:  "2.1.3",
			expectErr:      badControllerSelected.Error(),
		},
	}

	s.assertUpgradeTests(c, tests, upgradeCommand)
}

func (s *UpgradeIAASControllerSuite) TestUpgradeDryRun(c *gc.C) {
	s.assertUpgradeDryRun(c, "upgrade-controller", s.upgradeControllerCommand)
}

func (s *UpgradeIAASControllerSuite) TestUpgradeWrongPermissions(c *gc.C) {
	details, err := s.ControllerStore.AccountDetails("kontroll")
	c.Assert(err, jc.ErrorIsNil)
	details.LastKnownAccess = string(permission.ReadAccess)
	err = s.ControllerStore.UpdateAccount("kontroll", *details)
	c.Assert(err, jc.ErrorIsNil)
	com := s.upgradeControllerCommand()
	err = cmdtesting.InitCommand(com, []string{})
	c.Assert(err, jc.ErrorIsNil)
	ctx := cmdtesting.Context(c)
	err = com.Run(ctx)
	expectedErrMsg := fmt.Sprintf("upgrade not possible missing"+
		" permissions, current level %q, need: %q", details.LastKnownAccess, permission.SuperuserAccess)
	c.Assert(err, gc.ErrorMatches, expectedErrMsg)
}

func (s *UpgradeIAASControllerSuite) TestUpgradeDifferentUser(c *gc.C) {
	user, err := s.BackingState.AddUser("rick", "rick", "dummy-secret", "admin")
	c.Assert(err, jc.ErrorIsNil)

	s.authoriser = apiservertesting.FakeAuthorizer{
		Tag: user.Tag(),
	}
	ctag := names.NewControllerTag(s.BackingState.ControllerUUID())

	_, err = s.BackingState.SetUserAccess(user.UserTag(), ctag, permission.SuperuserAccess)
	c.Assert(err, jc.ErrorIsNil)

	err = s.ControllerStore.UpdateAccount("kontroll", jujuclient.AccountDetails{
		User:            "rick",
		LastKnownAccess: string(permission.SuperuserAccess),
		Password:        "dummy-secret",
	})
	c.Assert(err, jc.ErrorIsNil)

	cmd := &upgradeControllerCommand{
		baseUpgradeCommand: baseUpgradeCommand{},
	}
	cmd.SetClientStore(s.ControllerStore)
	cmdrun := modelcmd.WrapController(cmd)
	_, err = cmdtesting.RunCommand(c, cmdrun)
	c.Assert(err, jc.ErrorIsNil)
}

type UpgradeCAASControllerSuite struct {
	UpgradeBaseSuite
}

func (s *UpgradeCAASControllerSuite) SetUpTest(c *gc.C) {
	s.UpgradeBaseSuite.SetUpTest(c)
	err := s.ControllerStore.RemoveModel(jujutesting.ControllerName, "admin/controller")
	c.Assert(err, jc.ErrorIsNil)
	err = s.ControllerStore.UpdateModel(jujutesting.ControllerName, "admin/controller", jujuclient.ModelDetails{
		ModelType: model.CAAS,
		ModelUUID: coretesting.ModelTag.Id(),
	})
	c.Assert(err, jc.ErrorIsNil)

	s.resources = common.NewResources()
	s.authoriser = apiservertesting.FakeAuthorizer{
		Tag: s.AdminUserTag(c),
	}

	s.CmdBlockHelper = coretesting.NewCmdBlockHelper(s.APIState)
	c.Assert(s.CmdBlockHelper, gc.NotNil)
	s.AddCleanup(func(*gc.C) { s.CmdBlockHelper.Close() })
}

var _ = gc.Suite(&UpgradeCAASControllerSuite{})

var upgradeCAASControllerTests = []upgradeTest{{
	about:          "unwanted extra argument",
	currentVersion: "1.0.0",
	agentVersion:   "1.0.0",
	args:           []string{"foo"},
	expectInitErr:  "unrecognized args:.*",
}, {
	about:          "invalid --agent-version value",
	currentVersion: "1.0.0",
	agentVersion:   "1.0.0",
	args:           []string{"--agent-version", "invalid-version"},
	expectInitErr:  "invalid version .*",
}, {
	about:          "latest supported stable release",
	available:      []string{"2.1.0", "2.1.2", "2.1.3", "2.1.3.666", "2.1-dev1"},
	streams:        []string{"2.1.0-groovy-amd64", "2.1.2-groovy-amd64", "2.1.3-groovy-amd64", "2.1-dev1-groovy-amd64"},
	currentVersion: "2.0.0",
	agentVersion:   "2.0.0",
	expectVersion:  "2.1.3",
}, {
	about:          "latest supported stable release increments by one minor version number",
	available:      []string{"1.21.3", "1.22.1"},
	streams:        []string{"1.21.3-groovy-amd64", "1.22.1-groovy-amd64"},
	currentVersion: "1.22.1",
	agentVersion:   "1.20.14",
	expectVersion:  "1.21.3",
}, {
	about:          "latest supported stable release from custom version",
	available:      []string{"1.21.3", "1.22.1"},
	streams:        []string{"1.21.3-groovy-amd64", "1.22.1-groovy-amd64"},
	currentVersion: "1.22.1",
	agentVersion:   "1.20.14.1",
	expectVersion:  "1.21.3",
}, {
	about:          "fallback to released if streams not available",
	available:      []string{"1.21.3", "1.21.4", "1.22-beta1"},
	currentVersion: "1.21.3",
	agentVersion:   "1.21.3",
	expectVersion:  "1.21.4",
}}

func (s *UpgradeCAASControllerSuite) upgradeControllerCommand(
	controllerAPI ControllerAPI, modelManagerAPI ModelManagerAPI,
) cmd.Command {
	cmd := &upgradeControllerCommand{}
	cmd.controllerAPI = controllerAPI
	cmd.modelManagerAPI = modelManagerAPI
	cmd.SetClientStore(s.ControllerStore)
	return modelcmd.WrapController(cmd)
}

func (s *UpgradeCAASControllerSuite) TestUpgrade(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	controllerAPI := mocks.NewMockControllerAPI(ctrl)
	modelManagerAPI := mocks.NewMockModelManagerAPI(ctrl)

	assertAndMocks := func(tagsInfo []tagInfo) {
		gomock.InOrder(
			modelManagerAPI.EXPECT().ToolVersions().DoAndReturn(
				func() (coretools.List, error) {
					var tags coretools.List
					for _, t := range tagsInfo {
						v, err := version.Parse(t.Tag)
						c.Check(err, jc.ErrorIsNil)
						tags = append(tags, &coretools.Tools{
							Version: version.Binary{
								Number:  v,
								Release: coreos.HostOSTypeName(),
								Arch:    "amd64",
							},
						})
					}
					return tags, nil
				},
			),
			controllerAPI.EXPECT().Close().Return(nil),
		)
	}

	s.assertUpgradeTests(c, upgradeCAASControllerTests, assertAndMocks, func() cmd.Command {
		return s.upgradeControllerCommand(controllerAPI, modelManagerAPI)
	})
}

func (s *UpgradeCAASControllerSuite) assertUpgradeTests(
	c *gc.C, tests []upgradeTest,
	assertAndMocks func(tagsInfo []tagInfo),
	upgradeJujuCommand upgradeCommandFunc,
) {
	for i, test := range tests {
		c.Logf("\ntest %d: %s", i, test.about)
		s.Reset(c)
		tools.DefaultBaseURL = ""
		err := s.ControllerStore.RemoveModel(jujutesting.ControllerName, "admin/controller")
		c.Assert(err, jc.ErrorIsNil)
		err = s.ControllerStore.UpdateModel(jujutesting.ControllerName, "admin/controller", jujuclient.ModelDetails{
			ModelType: model.CAAS,
			ModelUUID: coretesting.ModelTag.Id(),
		})
		c.Assert(err, jc.ErrorIsNil)

		s.setUpEnvAndTools(c, test.currentVersion+"-groovy-amd64", test.agentVersion, test.streams)

		// Set up apparent CLI version and initialize the command.
		current := version.MustParse(test.currentVersion)
		s.PatchValue(&jujuversion.Current, current)
		com := upgradeJujuCommand()
		if err := cmdtesting.InitCommand(com, test.args); err != nil {
			if test.expectInitErr != "" {
				c.Check(err, gc.ErrorMatches, test.expectInitErr)
			} else {
				c.Check(err, jc.ErrorIsNil)
			}
			continue
		}

		// Set up state and environ, and run the command.
		updateAttrs := map[string]interface{}{
			"agent-version": test.agentVersion,
		}
		err = s.Model.UpdateModelConfig(updateAttrs, nil)
		c.Assert(err, jc.ErrorIsNil)
		tagsInfo := make([]tagInfo, len(test.available))
		for i, v := range test.available {
			tagsInfo[i] = tagInfo{v}
		}
		assertAndMocks(tagsInfo)

		err = com.Run(cmdtesting.Context(c))
		if test.expectErr != "" {
			c.Check(err, gc.ErrorMatches, test.expectErr)
			continue
		} else if !c.Check(err, jc.ErrorIsNil) {
			continue
		}

		// Check expected changes to environ/state.
		cfg, err := s.Model.ModelConfig()
		c.Check(err, jc.ErrorIsNil)
		agentVersion, ok := cfg.AgentVersion()
		c.Check(ok, jc.IsTrue)
		c.Check(agentVersion, gc.Equals, version.MustParse(test.expectVersion))
	}
}
