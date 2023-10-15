// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"fmt"

	"github.com/juju/cmd/v3"
	"github.com/juju/cmd/v3/cmdtesting"
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version/v2"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/cmd/juju/commands/mocks"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/permission"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/jujuclient/jujuclienttesting"
	coretesting "github.com/juju/juju/testing"
	jujuversion "github.com/juju/juju/version"
)

type UpgradeControllerBaseSuite struct {
	UpgradeBaseSuite
}

func (s *UpgradeControllerBaseSuite) SetUpTest(c *gc.C) {
	s.UpgradeBaseSuite.SetUpTest(c)
	s.resources = common.NewResources()
	s.authoriser = apiservertesting.FakeAuthorizer{
		Tag: s.AdminUserTag(c),
	}
	s.CmdBlockHelper = coretesting.NewCmdBlockHelper(s.APIState)
	c.Assert(s.CmdBlockHelper, gc.NotNil)
	s.AddCleanup(func(*gc.C) { s.CmdBlockHelper.Close() })
}

var upgradeIAASControllerPassthroughTests = []upgradeTest{
	{
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
	},
}

func (s *UpgradeControllerBaseSuite) upgradeControllerCommand(c *gc.C, _ *upgradeTest) (*gomock.Controller, cmd.Command) {
	ctrl := gomock.NewController(c)
	s.modelUpgrader = mocks.NewMockModelUpgraderAPI(ctrl)
	s.modelUpgrader.EXPECT().BestAPIVersion().AnyTimes().Return(0)
	s.modelUpgrader.EXPECT().Close().AnyTimes()
	cmd := &upgradeControllerCommand{
		baseUpgradeCommand: baseUpgradeCommand{
			modelUpgraderAPI: s.modelUpgrader,
		},
	}
	cmd.SetClientStore(s.ControllerStore)
	return ctrl, modelcmd.WrapController(cmd)
}

func (s *UpgradeControllerBaseSuite) TestUpgradeWithRealUpload(c *gc.C) {
	s.Reset(c)
	s.PatchValue(&jujuversion.Current, version.MustParse("1.99.99"))
	_, cmd := s.upgradeControllerCommand(c, nil)
	_, err := cmdtesting.RunCommand(c, cmd, "--build-agent")
	c.Assert(err, jc.ErrorIsNil)
	vers := coretesting.CurrentVersion()
	vers.Build = 1
	s.checkToolsUploaded(c, vers, vers.Number)
}

func (s *UpgradeControllerBaseSuite) TestUpgradeCorrectControllerLegacy(c *gc.C) {
	// TODO(juju3) - remove me
	badControllerName := "not-the-right-controller"
	badControllerSelected := errors.New("bad controller selected")
	upgradeCommand := func(c *gc.C, test *upgradeTest) (*gomock.Controller, cmd.Command) {
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
		return s.upgradeControllerCommand(c, test)
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

	s.assertUpgradeTestsLegacy(c, tests, upgradeCommand)
}

func (s *UpgradeControllerBaseSuite) TestUpgradeDryRunLegacy(c *gc.C) {
	s.assertUpgradeDryRunLegacy(c, "upgrade-controller", s.upgradeControllerCommand)
}

func (s *UpgradeControllerBaseSuite) TestUpgradeWrongPermissions(c *gc.C) {
	details, err := s.ControllerStore.AccountDetails("kontroll")
	c.Assert(err, jc.ErrorIsNil)
	details.LastKnownAccess = string(permission.ReadAccess)
	err = s.ControllerStore.UpdateAccount("kontroll", *details)
	c.Assert(err, jc.ErrorIsNil)
	_, com := s.upgradeControllerCommand(c, nil)
	err = cmdtesting.InitCommand(com, []string{})
	c.Assert(err, jc.ErrorIsNil)
	ctx := cmdtesting.Context(c)
	err = com.Run(ctx)
	expectedErrMsg := fmt.Sprintf("upgrade not possible missing"+
		" permissions, current level %q, need: %q", details.LastKnownAccess, permission.SuperuserAccess)
	c.Assert(err, gc.ErrorMatches, expectedErrMsg)
}

func (s *UpgradeControllerBaseSuite) TestUpgradeDifferentUser(c *gc.C) {
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

type UpgradeIAASControllerSuite struct {
	UpgradeControllerBaseSuite
}

var _ = gc.Suite(&UpgradeIAASControllerSuite{})

func (s *UpgradeIAASControllerSuite) SetUpTest(c *gc.C) {
	s.UpgradeControllerBaseSuite.SetUpTest(c)
	err := s.ControllerStore.RemoveModel(jujutesting.ControllerName, "admin/controller")
	c.Assert(err, jc.ErrorIsNil)
	err = s.ControllerStore.UpdateModel(jujutesting.ControllerName, "admin/controller", jujuclient.ModelDetails{
		ModelType: model.IAAS,
		ModelUUID: coretesting.ModelTag.Id(),
	})
	c.Assert(err, jc.ErrorIsNil)

	// Run a subset of the upgrade-juju tests ensuring the controller
	// model is selected.
	c.Assert(s.Model.Name(), gc.Equals, "controller")
	err = s.ControllerStore.SetCurrentModel("kontroll", "")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *UpgradeIAASControllerSuite) TestUpgradeLegacy(c *gc.C) {
	s.assertUpgradeTestsLegacy(c, upgradeIAASControllerPassthroughTests, s.upgradeControllerCommand)
}

type UpgradeCAASControllerSuite struct {
	UpgradeBaseSuite
}

var _ = gc.Suite(&UpgradeCAASControllerSuite{})

func (s *UpgradeCAASControllerSuite) SetUpTest(c *gc.C) {
	s.UpgradeBaseSuite.SetUpTest(c)
	err := s.ControllerStore.RemoveModel(jujutesting.ControllerName, "admin/controller")
	c.Assert(err, jc.ErrorIsNil)
	err = s.ControllerStore.UpdateModel(jujutesting.ControllerName, "admin/controller", jujuclient.ModelDetails{
		ModelType: model.CAAS,
		ModelUUID: coretesting.ModelTag.Id(),
	})
	c.Assert(err, jc.ErrorIsNil)

	// Run a subset of the upgrade-juju tests ensuring the controller
	// model is selected.
	c.Assert(s.Model.Name(), gc.Equals, "controller")
	err = s.ControllerStore.SetCurrentModel("kontroll", "")
	c.Assert(err, jc.ErrorIsNil)
}

var upgradeCAASControllerTests = []upgradeTest{{
	about:          "unwanted extra argument",
	currentVersion: "1.0.0-ubuntu-amd64",
	agentVersion:   "1.0.0",
	args:           []string{"foo"},
	expectInitErr:  "unrecognized args:.*",
}, {
	about:          "invalid --agent-version value",
	currentVersion: "1.0.0-ubuntu-amd64",
	agentVersion:   "1.0.0",
	args:           []string{"--agent-version", "invalid-version"},
	expectInitErr:  "invalid version .*",
}, {
	about:          "latest supported stable release",
	available:      []string{"2.1.0-ubuntu-amd64", "2.1.2-ubuntu-amd64", "2.1.3-ubuntu-amd64", "2.1-dev1-ubuntu-amd64"},
	currentVersion: "2.0.0-ubuntu-amd64",
	agentVersion:   "2.0.0",
	expectVersion:  "2.1.3",
}, {
	about:          "latest supported stable release increments by one minor version number",
	available:      []string{"1.21.3-ubuntu-amd64", "1.22.1-ubuntu-amd64"},
	currentVersion: "1.22.1-ubuntu-amd64",
	agentVersion:   "1.20.14",
	expectVersion:  "1.21.3",
}, {
	about:          "latest supported stable release from custom version",
	available:      []string{"1.21.3-ubuntu-amd64", "1.22.1-ubuntu-amd64"},
	currentVersion: "1.22.1-ubuntu-amd64",
	agentVersion:   "1.20.14.1",
	expectVersion:  "1.21.3",
}, {
	about:          "fallback to released if streams not available",
	available:      []string{"1.21.3-ubuntu-amd64", "1.21.4-ubuntu-amd64"},
	currentVersion: "1.21.3-ubuntu-amd64",
	agentVersion:   "1.21.3",
	expectVersion:  "1.21.4",
}}

func (s *UpgradeCAASControllerSuite) upgradeModelCommand(*gc.C, *upgradeTest) (*gomock.Controller, cmd.Command) {
	return nil, newUpgradeJujuCommandForTest(s.ControllerStore, nil, nil, nil, nil, nil)
}

func (s *UpgradeCAASControllerSuite) TestUpgradeLegacy(c *gc.C) {
	s.UpgradeBaseSuite.assertUpgradeTestsLegacy(c, upgradeCAASControllerTests, s.upgradeModelCommand)
}
