// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/juju/cmd"
	"github.com/juju/cmd/cmdtesting"
	"github.com/juju/errors"
	"github.com/juju/os/series"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/arch"
	"github.com/juju/version"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v3"

	"github.com/juju/juju/apiserver/common"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/docker"
	"github.com/juju/juju/environs/tools"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/jujuclient/jujuclienttesting"
	coretesting "github.com/juju/juju/testing"
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
	currentVersion: "1.0.0-quantal-amd64",
	args:           []string{"foo"},
	expectInitErr:  "unrecognized args:.*",
}, {
	about:          "invalid --agent-version value",
	currentVersion: "1.0.0-quantal-amd64",
	args:           []string{"--agent-version", "invalid-version"},
	expectInitErr:  "invalid version .*",
}, {
	about:          "latest supported stable release",
	available:      []string{"2.1.0-quantal-amd64", "2.1.2-quantal-i386", "2.1.3-quantal-amd64", "2.1-dev1-quantal-amd64"},
	currentVersion: "2.0.0-quantal-amd64",
	agentVersion:   "2.0.0",
	expectVersion:  "2.1.3",
}, {
	about:          "latest supported stable, when client is dev, explicit upload",
	available:      []string{"2.1-dev1-quantal-amd64", "2.1.0-quantal-amd64", "2.3-dev0-quantal-amd64", "3.0.1-quantal-amd64"},
	currentVersion: "2.1-dev0-quantal-amd64",
	agentVersion:   "2.0.0",
	args:           []string{"--build-agent"},
	expectVersion:  "2.1-dev0.1",
}, {
	about:          "upload with explicit version",
	currentVersion: "2.2.0-quantal-amd64",
	agentVersion:   "2.0.0",
	args:           []string{"--build-agent", "--agent-version", "2.7.3"},
	expectVersion:  "2.7.3.1",
	expectUploaded: []string{"2.7.3.1-quantal-amd64", "2.7.3.1-%LTS%-amd64", "2.7.3.1-raring-amd64"},
}}

func (s *UpgradeIAASControllerSuite) upgradeControllerCommand(minUpgradeVers map[int]version.Number) cmd.Command {
	cmd := &upgradeControllerCommand{
		baseUpgradeCommand: baseUpgradeCommand{minMajorUpgradeVersion: minMajorUpgradeVersion},
	}
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
	cmd := s.upgradeControllerCommand(map[int]version.Number{2: version.MustParse("1.99.99")})
	_, err := cmdtesting.RunCommand(c, cmd, "--build-agent")
	c.Assert(err, jc.ErrorIsNil)
	vers := version.Binary{
		Number: jujuversion.Current,
		Arch:   arch.HostArch(),
		Series: series.MustHostSeries(),
	}
	vers.Build = 1
	s.checkToolsUploaded(c, vers, vers.Number)
}

func (s *UpgradeIAASControllerSuite) TestUpgradeCorrectController(c *gc.C) {
	badControllerName := "not-the-right-controller"
	badControllerSelected := errors.New("bad controller selected")
	upgradeCommand := func(minUpgradeVers map[int]version.Number) cmd.Command {
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
		cmd := &upgradeControllerCommand{
			baseUpgradeCommand: baseUpgradeCommand{minMajorUpgradeVersion: minMajorUpgradeVersion},
		}
		cmd.SetClientStore(s.ControllerStore)
		return modelcmd.WrapController(cmd)
	}

	tests := []upgradeTest{
		{
			about:          "latest supported stable release with specified controller",
			available:      []string{"2.1.0-quantal-amd64", "2.1.2-quantal-i386", "2.1.3-quantal-amd64", "2.1-dev1-quantal-amd64"},
			currentVersion: "2.0.0-quantal-amd64",
			agentVersion:   "2.0.0",
			expectVersion:  "2.1.3",
			args:           []string{"--controller", "kontroll"},
		},
		{
			about:          "latest supported stable release without specified controller",
			available:      []string{"2.1.0-quantal-amd64", "2.1.2-quantal-i386", "2.1.3-quantal-amd64", "2.1-dev1-quantal-amd64"},
			currentVersion: "2.0.0-quantal-amd64",
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
	com := s.upgradeControllerCommand(nil)
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
		baseUpgradeCommand: baseUpgradeCommand{minMajorUpgradeVersion: minMajorUpgradeVersion},
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
	available:      []string{"2.1.0", "2.1.2", "2.1.3", "2.1-dev1"},
	streams:        []string{"2.1.0-quantal-amd64", "2.1.2-quantal-amd64", "2.1.3-quantal-amd64", "2.1-dev1-quantal-amd64"},
	currentVersion: "2.0.0",
	agentVersion:   "2.0.0",
	expectVersion:  "2.1.3",
}, {
	about:          "latest supported stable release increments by one minor version number",
	available:      []string{"1.21.3", "1.22.1"},
	streams:        []string{"1.21.3-quantal-amd64", "1.22.1-quantal-amd64"},
	currentVersion: "1.22.1",
	agentVersion:   "1.20.14",
	expectVersion:  "1.21.3",
}, {
	about:          "latest supported stable release from custom version",
	available:      []string{"1.21.3", "1.22.1"},
	streams:        []string{"1.21.3-quantal-amd64", "1.22.1-quantal-amd64"},
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

func (s *UpgradeCAASControllerSuite) upgradeControllerCommand(_ map[int]version.Number) cmd.Command {
	cmd := &upgradeControllerCommand{}
	cmd.SetClientStore(s.ControllerStore)
	return modelcmd.WrapController(cmd)
}

func (s *UpgradeCAASControllerSuite) TestUpgrade(c *gc.C) {
	c.Assert(s.Model.Name(), gc.Equals, "controller")
	err := s.ControllerStore.SetCurrentModel("kontroll", "")
	c.Assert(err, jc.ErrorIsNil)

	s.assertUpgradeTests(c, upgradeCAASControllerTests, s.upgradeControllerCommand)
}

func (s *UpgradeCAASControllerSuite) assertUpgradeTests(c *gc.C, tests []upgradeTest, upgradeJujuCommand upgradeCommandFunc) {
	type info struct {
		Tag string `json:"name"`
	}
	var tagInfo []info

	s.PatchValue(&docker.HttpGet, func(url string, timeout time.Duration) ([]byte, error) {
		c.Assert(url, gc.Equals, "https://registry.hub.docker.com/v1/repositories/jujusolutions/jujud-operator/tags")
		c.Assert(timeout, gc.Equals, 30*time.Second)
		return json.Marshal(tagInfo)
	})

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

		s.setUpEnvAndTools(c, test.currentVersion+"-quantal-amd64", test.agentVersion, test.streams)

		// Set up apparent CLI version and initialize the command.
		current := version.MustParse(test.currentVersion)
		s.PatchValue(&jujuversion.Current, current)
		com := upgradeJujuCommand(nil)
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
		tagInfo = make([]info, len(test.available))
		for i, v := range test.available {
			tagInfo[i] = info{v}
		}

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
