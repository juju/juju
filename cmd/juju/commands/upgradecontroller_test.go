// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"github.com/juju/cmd"
	"github.com/juju/cmd/cmdtesting"
	"github.com/juju/os/series"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/arch"
	"github.com/juju/version"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/core/model"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/jujuclient"
	coretesting "github.com/juju/juju/testing"
	jujuversion "github.com/juju/juju/version"
)

type UpgradeControllerSuite struct {
	UpgradeBaseSuite
}

func (s *UpgradeControllerSuite) SetUpTest(c *gc.C) {
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

var _ = gc.Suite(&UpgradeControllerSuite{})

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
	tools:          []string{"2.1.0-quantal-amd64", "2.1.2-quantal-i386", "2.1.3-quantal-amd64", "2.1-dev1-quantal-amd64"},
	currentVersion: "2.0.0-quantal-amd64",
	agentVersion:   "2.0.0",
	expectVersion:  "2.1.3",
}, {
	about:          "latest supported stable, when client is dev, explicit upload",
	tools:          []string{"2.1-dev1-quantal-amd64", "2.1.0-quantal-amd64", "2.3-dev0-quantal-amd64", "3.0.1-quantal-amd64"},
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

func (s *UpgradeControllerSuite) upgradeControllerCommand(minUpgradeVers map[int]version.Number) cmd.Command {
	cmd := &upgradeControllerCommand{}
	cmd.SetClientStore(s.ControllerStore)
	return modelcmd.WrapController(cmd)
}

func (s *UpgradeControllerSuite) TestUpgradeIAASController(c *gc.C) {
	// Run a subset of the upgrade-juju tests ensuring the controller
	// model is selected.
	c.Assert(s.Model.Name(), gc.Equals, "controller")
	err := s.ControllerStore.SetCurrentModel("kontroll", "")
	c.Assert(err, jc.ErrorIsNil)

	s.assertUpgradeTests(c, upgradeIAASControllerPassthroughTests, s.upgradeControllerCommand)
}

func (s *UpgradeControllerSuite) TestUpgradeJujuWithRealUpload(c *gc.C) {
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

func (s *UpgradeControllerSuite) TestUpgradeDryRun(c *gc.C) {
	s.assertUpgradeDryRun(c, "upgrade-controller", s.upgradeControllerCommand)
}
