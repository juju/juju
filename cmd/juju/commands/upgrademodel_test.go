// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"fmt"

	"github.com/juju/cmd/v4"
	"github.com/juju/cmd/v4/cmdtesting"
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version/v2"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/commands/mocks"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/environs/sync"
	toolstesting "github.com/juju/juju/environs/tools/testing"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/rpc/params"
	coretesting "github.com/juju/juju/testing"
	jujuversion "github.com/juju/juju/version"
)

func newUpgradeJujuCommandForTest(
	store jujuclient.ClientStore,
	modelConfigAPI ModelConfigAPI,
	modelUpgrader ModelUpgraderAPI,
	controllerAPI ControllerAPI,
	options ...modelcmd.WrapOption,
) cmd.Command {
	command := &upgradeJujuCommand{
		baseUpgradeCommand: baseUpgradeCommand{
			modelConfigAPI:   modelConfigAPI,
			modelUpgraderAPI: modelUpgrader,
			controllerAPI:    controllerAPI,
		},
	}
	command.SetClientStore(store)
	return modelcmd.Wrap(command, options...)
}

type upgradeNewSuite struct {
	testing.IsolationSuite

	modelConfigAPI *mocks.MockModelConfigAPI
	modelUpgrader  *mocks.MockModelUpgraderAPI
	controllerAPI  *mocks.MockControllerAPI
	store          *mocks.MockClientStore
}

var _ = gc.Suite(&upgradeNewSuite{})

func (s *upgradeNewSuite) upgradeJujuCommand(c *gc.C, isCAAS bool) (*gomock.Controller, cmd.Command) {
	ctrl := gomock.NewController(c)
	s.modelConfigAPI = mocks.NewMockModelConfigAPI(ctrl)
	s.modelUpgrader = mocks.NewMockModelUpgraderAPI(ctrl)
	s.controllerAPI = mocks.NewMockControllerAPI(ctrl)
	s.store = mocks.NewMockClientStore(ctrl)

	s.modelConfigAPI.EXPECT().Close().AnyTimes()
	s.modelUpgrader.EXPECT().Close().AnyTimes()
	s.controllerAPI.EXPECT().Close().AnyTimes()

	s.store.EXPECT().CurrentController().AnyTimes().Return("c-1", nil)
	s.store.EXPECT().ControllerByName("c-1").AnyTimes().Return(&jujuclient.ControllerDetails{
		APIEndpoints: []string{"0.1.2.3:1234"},
	}, nil)
	s.store.EXPECT().CurrentModel("c-1").AnyTimes().Return("m-1", nil)
	s.store.EXPECT().AccountDetails("c-1").AnyTimes().Return(&jujuclient.AccountDetails{User: "foo"}, nil)
	cookieJar := mocks.NewMockCookieJar(ctrl)
	cookieJar.EXPECT().Save().AnyTimes().Return(nil)
	s.store.EXPECT().CookieJar("c-1").AnyTimes().Return(cookieJar, nil)

	modelType := model.IAAS
	if isCAAS {
		modelType = model.CAAS
	}

	s.store.EXPECT().ModelByName("c-1", "foo/m-1").AnyTimes().Return(&jujuclient.ModelDetails{
		ModelUUID: coretesting.ModelTag.Id(),
		ModelType: modelType,
	}, nil)

	return ctrl, newUpgradeJujuCommandForTest(s.store,
		s.modelConfigAPI, s.modelUpgrader, s.controllerAPI,
	)
}

func (s *upgradeNewSuite) TestUpgradeModelFailedCAASWithBuildAgent(c *gc.C) {
	ctrl, cmd := s.upgradeJujuCommand(c, true)
	defer ctrl.Finish()

	_, err := cmdtesting.RunCommand(c, cmd, `--build-agent`)
	c.Assert(err, gc.ErrorMatches, `--build-agent for k8s model upgrades not supported`)
}

func (s *upgradeNewSuite) TestUpgradeModelProvidedAgentVersionUpToDate(c *gc.C) {
	ctrl, cmd := s.upgradeJujuCommand(c, false)
	defer ctrl.Finish()

	cfg := coretesting.FakeConfig().Merge(coretesting.Attrs{
		"agent-version": coretesting.FakeVersionNumber.String(),
	})

	s.modelConfigAPI.EXPECT().ModelGet().Return(cfg, nil)

	ctx, err := cmdtesting.RunCommand(c, cmd, "--agent-version", coretesting.FakeVersionNumber.String())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(ctx), gc.Equals, "no upgrades available\n")
}

func (s *upgradeNewSuite) TestUpgradeModelFailedNoPermissionForControllerModelWithBuildAgent(c *gc.C) {
	ctrl, cmd := s.upgradeJujuCommand(c, false)
	defer ctrl.Finish()

	cfg := coretesting.FakeConfig().Merge(coretesting.Attrs{
		"agent-version": coretesting.FakeVersionNumber.String(),
	})

	gomock.InOrder(
		s.modelConfigAPI.EXPECT().ModelGet().Return(cfg, nil),
		s.controllerAPI.EXPECT().ModelConfig().Return(nil, &params.Error{Code: params.CodeUnauthorized}),
	)

	_, err := cmdtesting.RunCommand(c, cmd, `--build-agent`)
	c.Assert(err, gc.ErrorMatches, `--build-agent can only be used with the controller model but you don't have permission to access that model`)
}

func (s *upgradeNewSuite) TestUpgradeModelFailedForNonControllerModelWithBuildAgent(c *gc.C) {
	ctrl, cmd := s.upgradeJujuCommand(c, false)
	defer ctrl.Finish()

	modelCfg := coretesting.FakeConfig().Merge(coretesting.Attrs{
		"agent-version": coretesting.FakeVersionNumber.String(),
	})

	controllerCfg := coretesting.FakeConfig().Merge(coretesting.Attrs{
		"uuid": coretesting.ControllerTag.Id(),
	})

	gomock.InOrder(
		s.modelConfigAPI.EXPECT().ModelGet().Return(modelCfg, nil),
		s.controllerAPI.EXPECT().ModelConfig().Return(controllerCfg, nil),
	)

	_, err := cmdtesting.RunCommand(c, cmd, `--build-agent`)
	c.Assert(err, gc.ErrorMatches, `--build-agent can only be used with the controller model`)
}

func (s *upgradeNewSuite) TestUpgradeModelFailedWithBuildAgentAndAgentVersion(c *gc.C) {
	ctrl, cmd := s.upgradeJujuCommand(c, false)
	defer ctrl.Finish()

	cfg := coretesting.FakeConfig().Merge(coretesting.Attrs{
		"agent-version": coretesting.FakeVersionNumber.String(),
	})

	gomock.InOrder(
		s.modelConfigAPI.EXPECT().ModelGet().Return(cfg, nil),
		s.controllerAPI.EXPECT().ModelConfig().Return(cfg, nil),
	)

	_, err := cmdtesting.RunCommand(c, cmd,
		"--build-agent",
		"--agent-version", version.MustParse("3.9.99").String(),
	)
	c.Assert(err, gc.ErrorMatches, `--build-agent cannot be used with --agent-version together`)
}

func (s *upgradeNewSuite) TestUpgradeModelWithAgentVersion(c *gc.C) {
	ctrl, cmd := s.upgradeJujuCommand(c, false)
	defer ctrl.Finish()

	cfg := coretesting.FakeConfig().Merge(coretesting.Attrs{
		// TODO (hml) 19-oct-2022
		// Once upgrade from 2.9 to 3.0 is supported, go back to
		// using coretesting.FakeVersionNumber.String() in this
		// test.
		//"agent-version": coretesting.FakeVersionNumber.String(),
		"agent-version": "3.0.1",
	})

	gomock.InOrder(
		s.modelConfigAPI.EXPECT().ModelGet().Return(cfg, nil),
		s.controllerAPI.EXPECT().ModelConfig().Return(cfg, nil),
		s.modelUpgrader.EXPECT().UpgradeModel(
			coretesting.ModelTag.Id(), version.MustParse("3.9.99"),
			"", false, false,
		).Return(version.MustParse("3.9.99"), nil),
	)

	ctx, err := cmdtesting.RunCommand(c, cmd,
		"--agent-version", version.MustParse("3.9.99").String(),
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(ctx), gc.Equals, `
best version:
    3.9.99
`[1:])
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, `
started upgrade to 3.9.99
`[1:])
}

func (s *upgradeNewSuite) TestUpgradeModelWithAgentVersionUploadLocalOfficial(c *gc.C) {
	s.reset(c)

	s.PatchValue(&jujuversion.Current, func() version.Number {
		v := jujuversion.Current
		v.Build = 0
		return v
	}())

	s.PatchValue(&CheckCanImplicitUpload,
		func(model.ModelType, bool, version.Number, version.Number) bool { return true },
	)

	ctrl, cmd := s.upgradeJujuCommand(c, false)
	defer ctrl.Finish()

	agentVersion := coretesting.FakeVersionNumber
	cfg := coretesting.FakeConfig().Merge(coretesting.Attrs{
		"agent-version": agentVersion.String(),
	})

	c.Assert(agentVersion.Build, gc.Equals, 0)
	builtVersion := coretesting.CurrentVersion()
	targetVersion := builtVersion.Number
	builtVersion.Build++
	gomock.InOrder(
		s.modelConfigAPI.EXPECT().ModelGet().Return(cfg, nil),
		s.controllerAPI.EXPECT().ModelConfig().Return(cfg, nil),
		s.modelUpgrader.EXPECT().UpgradeModel(
			coretesting.ModelTag.Id(), targetVersion,
			"", false, false,
		).Return(
			version.Zero,
			errors.NotFoundf("available agent tool, upload required"),
		),
		s.modelUpgrader.EXPECT().UploadTools(gomock.Any(), gomock.Any(), builtVersion).Return(nil, nil),
		s.modelUpgrader.EXPECT().UpgradeModel(
			coretesting.ModelTag.Id(), builtVersion.Number,
			"", false, false,
		).Return(builtVersion.Number, nil),
	)

	ctx, err := cmdtesting.RunCommand(c, cmd,
		"--agent-version", targetVersion.String(),
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(ctx), gc.Equals, fmt.Sprintf(`
best version:
    %s
`, builtVersion.Number)[1:])
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, fmt.Sprintf(`
no prepackaged agent binaries available, using the local snap jujud %s
started upgrade to %s
`, builtVersion.Number, builtVersion.Number)[1:])
}

func (s *upgradeNewSuite) TestUpgradeModelWithAgentVersionAlreadyUpToDate(c *gc.C) {
	s.reset(c)

	ctrl, cmd := s.upgradeJujuCommand(c, false)
	defer ctrl.Finish()

	agentVersion := coretesting.FakeVersionNumber
	cfg := coretesting.FakeConfig().Merge(coretesting.Attrs{
		"agent-version": agentVersion.String(),
	})

	c.Assert(agentVersion.Build, gc.Equals, 0)
	targetVersion := coretesting.CurrentVersion()
	gomock.InOrder(
		s.modelConfigAPI.EXPECT().ModelGet().Return(cfg, nil),
		s.controllerAPI.EXPECT().ModelConfig().Return(cfg, nil),
		s.modelUpgrader.EXPECT().UpgradeModel(
			coretesting.ModelTag.Id(), targetVersion.ToPatch(),
			"", false, false,
		).Return(
			version.Zero,
			errors.AlreadyExistsf("up to date"),
		),
	)

	ctx, err := cmdtesting.RunCommand(c, cmd,
		"--agent-version", targetVersion.ToPatch().String(),
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(ctx), gc.Equals, "no upgrades available\n")
}

func (s *upgradeNewSuite) TestUpgradeModelWithAgentVersionFailedExpectUploadButWrongTargetVersion(c *gc.C) {
	s.reset(c)

	s.PatchValue(&CheckCanImplicitUpload,
		func(model.ModelType, bool, version.Number, version.Number) bool { return true },
	)

	ctrl, cmd := s.upgradeJujuCommand(c, false)
	defer ctrl.Finish()

	agentVersion := coretesting.FakeVersionNumber
	cfg := coretesting.FakeConfig().Merge(coretesting.Attrs{
		"agent-version": agentVersion.String(),
	})

	current := agentVersion
	current.Minor++ // local snap is newer.
	s.PatchValue(&jujuversion.Current, current)

	targetVersion := current
	targetVersion.Patch++ // wrong target version (It has to be equal to local snap version).
	c.Assert(targetVersion.Compare(current) == 0, jc.IsFalse)

	gomock.InOrder(
		s.modelConfigAPI.EXPECT().ModelGet().Return(cfg, nil),
		s.controllerAPI.EXPECT().ModelConfig().Return(cfg, nil),
		s.modelUpgrader.EXPECT().UpgradeModel(
			coretesting.ModelTag.Id(), targetVersion,
			"", false, false,
		).Return(
			version.Zero,
			errors.NotFoundf("available agent tool, upload required"),
		),
	)

	ctx, err := cmdtesting.RunCommand(c, cmd,
		"--agent-version", targetVersion.String(),
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(ctx), gc.Equals, "no upgrades available\n")
}

func (s *upgradeNewSuite) TestUpgradeModelWithAgentVersionExpectUploadFailedDueToNotAllowed(c *gc.C) {
	s.reset(c)

	s.PatchValue(&CheckCanImplicitUpload,
		func(model.ModelType, bool, version.Number, version.Number) bool { return false },
	)

	ctrl, cmd := s.upgradeJujuCommand(c, false)
	defer ctrl.Finish()

	agentVersion := coretesting.FakeVersionNumber
	cfg := coretesting.FakeConfig().Merge(coretesting.Attrs{
		"agent-version": agentVersion.String(),
	})

	targetVersion := coretesting.CurrentVersion().Number
	gomock.InOrder(
		s.modelConfigAPI.EXPECT().ModelGet().Return(cfg, nil),
		s.controllerAPI.EXPECT().ModelConfig().Return(cfg, nil),
		s.modelUpgrader.EXPECT().UpgradeModel(
			coretesting.ModelTag.Id(), targetVersion,
			"", false, false,
		).Return(
			version.Zero,
			errors.NotFoundf("available agent tool, upload required"),
		),
	)

	ctx, err := cmdtesting.RunCommand(c, cmd,
		"--agent-version", targetVersion.String(),
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(ctx), gc.Equals, "no upgrades available\n")
}

func (s *upgradeNewSuite) TestUpgradeModelWithAgentVersionExpectUploadFailedDueToNonControllerModel(c *gc.C) {
	s.reset(c)

	s.PatchValue(&CheckCanImplicitUpload,
		func(model.ModelType, bool, version.Number, version.Number) bool { return true },
	)

	ctrl, cmd := s.upgradeJujuCommand(c, false)
	defer ctrl.Finish()

	agentVersion := coretesting.FakeVersionNumber
	modelCfg := coretesting.FakeConfig().Merge(coretesting.Attrs{
		"agent-version": agentVersion.String(),
	})
	controllerCfg := coretesting.FakeConfig().Merge(coretesting.Attrs{
		"agent-version": agentVersion.String(),
		// This isn't right, but it's ok for testing because it's not important here.
		// We just need a different uuid.
		"uuid": coretesting.ControllerTag.Id(),
	})

	builtVersion := coretesting.CurrentVersion()
	targetVersion := builtVersion.Number
	gomock.InOrder(
		s.modelConfigAPI.EXPECT().ModelGet().Return(modelCfg, nil),
		s.controllerAPI.EXPECT().ModelConfig().Return(controllerCfg, nil),
		s.modelUpgrader.EXPECT().UpgradeModel(
			coretesting.ModelTag.Id(), targetVersion,
			"", false, false,
		).Return(
			version.Zero,
			errors.NotFoundf("available agent tool, upload required"),
		),
	)

	ctx, err := cmdtesting.RunCommand(c, cmd,
		"--agent-version", targetVersion.String(),
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(ctx), gc.Equals, "no upgrades available\n")
}

func (s *upgradeNewSuite) TestUpgradeModelWithAgentVersionDryRun(c *gc.C) {
	ctrl, cmd := s.upgradeJujuCommand(c, false)
	defer ctrl.Finish()

	cfg := coretesting.FakeConfig().Merge(coretesting.Attrs{
		// TODO (hml) 19-oct-2022
		// Once upgrade from 2.9 to 3.0 is supported, go back to
		// using coretesting.FakeVersionNumber.String() in this
		// test.
		//"agent-version": coretesting.FakeVersionNumber.String(),
		"agent-version": "3.0.1",
	})

	gomock.InOrder(
		s.modelConfigAPI.EXPECT().ModelGet().Return(cfg, nil),
		s.controllerAPI.EXPECT().ModelConfig().Return(cfg, nil),
		s.modelUpgrader.EXPECT().UpgradeModel(
			coretesting.ModelTag.Id(), version.MustParse("3.9.99"),
			"", false, true,
		).Return(version.MustParse("3.9.99"), nil),
	)

	ctx, err := cmdtesting.RunCommand(c, cmd,
		"--agent-version", version.MustParse("3.9.99").String(), "--dry-run",
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(ctx), gc.Equals, `
best version:
    3.9.99
upgrade to this version by running
    juju upgrade-model
`[1:])
}

func (s *upgradeNewSuite) TestUpgradeModelWithAgentVersionGotBlockers(c *gc.C) {
	ctrl, cmd := s.upgradeJujuCommand(c, false)
	defer ctrl.Finish()

	cfg := coretesting.FakeConfig().Merge(coretesting.Attrs{
		// TODO (hml) 19-oct-2022
		// Once upgrade from 2.9 to 3.0 is supported, go back to
		// using coretesting.FakeVersionNumber.String() in this
		// test.
		//"agent-version": coretesting.FakeVersionNumber.String(),
		"agent-version": "3.0.1",
	})

	gomock.InOrder(
		s.modelConfigAPI.EXPECT().ModelGet().Return(cfg, nil),
		s.controllerAPI.EXPECT().ModelConfig().Return(cfg, nil),
		s.modelUpgrader.EXPECT().UpgradeModel(
			coretesting.ModelTag.Id(), version.MustParse("3.9.99"),
			"", false, false,
		).Return(version.Zero, errors.New(`
cannot upgrade to "3.9.99" due to issues with these models:
"admin/default":
- the model hosts deprecated ubuntu machine(s): bionic(3) (not supported)
`[1:])),
	)

	_, err := cmdtesting.RunCommand(c, cmd,
		"--agent-version", version.MustParse("3.9.99").String(),
	)
	c.Assert(err.Error(), gc.Equals, `
cannot upgrade to "3.9.99" due to issues with these models:
"admin/default":
- the model hosts deprecated ubuntu machine(s): bionic(3) (not supported)
`[1:])
}

func (s *upgradeNewSuite) reset(c *gc.C) {
	s.PatchValue(&sync.BuildAgentTarball, toolstesting.GetMockBuildTools(c))
}

func (s *upgradeNewSuite) TestUpgradeModelWithBuildAgent(c *gc.C) {
	s.reset(c)

	ctrl, cmd := s.upgradeJujuCommand(c, false)
	defer ctrl.Finish()

	agentVersion := coretesting.FakeVersionNumber
	cfg := coretesting.FakeConfig().Merge(coretesting.Attrs{
		"agent-version": agentVersion.String(),
	})
	c.Assert(agentVersion.Build, gc.Equals, 0)
	builtVersion := coretesting.CurrentVersion()
	builtVersion.Build++
	gomock.InOrder(
		s.modelConfigAPI.EXPECT().ModelGet().Return(cfg, nil),
		s.controllerAPI.EXPECT().ModelConfig().Return(cfg, nil),
		s.modelUpgrader.EXPECT().UploadTools(gomock.Any(), gomock.Any(), builtVersion).Return(nil, nil),
		s.modelUpgrader.EXPECT().UpgradeModel(
			coretesting.ModelTag.Id(), builtVersion.Number,
			"", false, false,
		).Return(builtVersion.Number, nil),
	)

	ctx, err := cmdtesting.RunCommand(c, cmd, "--build-agent")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(ctx), gc.Equals, fmt.Sprintf(`
best version:
    %s
`, builtVersion.Number)[1:])
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, fmt.Sprintf(`
no prepackaged agent binaries available, using local agent binary %s (built from source)
started upgrade to %s
`, builtVersion.Number, builtVersion.Number)[1:])
}

func (s *upgradeNewSuite) TestUpgradeModelUpToDate(c *gc.C) {
	s.reset(c)

	ctrl, cmd := s.upgradeJujuCommand(c, false)
	defer ctrl.Finish()

	agentVersion := coretesting.FakeVersionNumber
	cfg := coretesting.FakeConfig().Merge(coretesting.Attrs{
		"agent-version": agentVersion.String(),
	})

	gomock.InOrder(
		s.modelConfigAPI.EXPECT().ModelGet().Return(cfg, nil),
		s.controllerAPI.EXPECT().ModelConfig().Return(cfg, nil),
		s.modelUpgrader.EXPECT().UpgradeModel(
			coretesting.ModelTag.Id(), version.Zero,
			"", false, false,
		).Return(version.Zero, errors.AlreadyExistsf("up to date")),
	)

	ctx, err := cmdtesting.RunCommand(c, cmd)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(ctx), gc.Equals, "no upgrades available\n")
}

func (s *upgradeNewSuite) TestUpgradeModelUpgradeToPublishedVersion(c *gc.C) {
	s.reset(c)

	ctrl, cmd := s.upgradeJujuCommand(c, false)
	defer ctrl.Finish()

	agentVersion := coretesting.FakeVersionNumber
	cfg := coretesting.FakeConfig().Merge(coretesting.Attrs{
		"agent-version": agentVersion.String(),
	})

	gomock.InOrder(
		s.modelConfigAPI.EXPECT().ModelGet().Return(cfg, nil),
		s.controllerAPI.EXPECT().ModelConfig().Return(cfg, nil),
		s.modelUpgrader.EXPECT().UpgradeModel(
			coretesting.ModelTag.Id(), version.Zero,
			"", false, false,
		).Return(version.MustParse("3.9.99"), nil),
	)

	ctx, err := cmdtesting.RunCommand(c, cmd)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(ctx), gc.Equals, `
best version:
    3.9.99
`[1:])
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, `
started upgrade to 3.9.99
`[1:])
}

func (s *upgradeNewSuite) TestUpgradeModelWithStream(c *gc.C) {
	s.reset(c)

	ctrl, cmd := s.upgradeJujuCommand(c, false)
	defer ctrl.Finish()

	agentVersion := coretesting.FakeVersionNumber
	cfg := coretesting.FakeConfig().Merge(coretesting.Attrs{
		"agent-version": agentVersion.String(),
	})

	gomock.InOrder(
		s.modelConfigAPI.EXPECT().ModelGet().Return(cfg, nil),
		s.controllerAPI.EXPECT().ModelConfig().Return(cfg, nil),
		s.modelUpgrader.EXPECT().UpgradeModel(
			coretesting.ModelTag.Id(), version.Zero,
			"proposed", false, false,
		).Return(version.MustParse("3.9.99"), nil),
	)

	ctx, err := cmdtesting.RunCommand(c, cmd, "--agent-stream", "proposed")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(ctx), gc.Equals, `
best version:
    3.9.99
`[1:])
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, `
started upgrade to 3.9.99
`[1:])
}

func (s *upgradeNewSuite) TestCheckCanImplicitUploadIAASModel(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	// Not IAAS model.
	canImplicitUpload := checkCanImplicitUpload(
		model.CAAS, true,
		version.MustParse("3.0.0"),
		version.MustParse("3.9.99.1"),
	)
	c.Check(canImplicitUpload, jc.IsFalse)

	// not official client.
	canImplicitUpload = checkCanImplicitUpload(
		model.IAAS, false,
		version.MustParse("3.9.99"),
		version.MustParse("3.0.0"),
	)
	c.Check(canImplicitUpload, jc.IsFalse)

	// non newer client.
	canImplicitUpload = checkCanImplicitUpload(
		model.IAAS, true,
		version.MustParse("2.9.99"),
		version.MustParse("3.0.0"),
	)
	c.Check(canImplicitUpload, jc.IsFalse)

	// client version with build number.
	canImplicitUpload = checkCanImplicitUpload(
		model.IAAS, true,
		version.MustParse("3.0.0.1"),
		version.MustParse("3.0.0"),
	)
	c.Check(canImplicitUpload, jc.IsTrue)

	// agent version with build number.
	canImplicitUpload = checkCanImplicitUpload(
		model.IAAS, true,
		version.MustParse("3.0.0"),
		version.MustParse("3.0.0.1"),
	)
	c.Check(canImplicitUpload, jc.IsTrue)

	// both client and agent version with build number == 0.
	canImplicitUpload = checkCanImplicitUpload(
		model.IAAS, true,
		version.MustParse("3.0.0"),
		version.MustParse("3.0.0"),
	)
	c.Check(canImplicitUpload, jc.IsFalse)
}
