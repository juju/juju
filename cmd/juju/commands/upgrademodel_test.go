// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"github.com/juju/errors"
	"github.com/juju/tc"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/cmd/juju/commands/mocks"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/semversion"
	jujuversion "github.com/juju/juju/core/version"
	"github.com/juju/juju/environs/sync"
	toolstesting "github.com/juju/juju/environs/tools/testing"
	"github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/internal/cmd/cmdtesting"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/jujuclient"
)

const (
	testControllerModelUUID = "0badf00d-0bad-400d-8000-4b1d0d06f00d"
)

func newUpgradeJujuCommandForTest(
	store jujuclient.ClientStore,
	modelConfigAPI ModelConfigAPI,
	modelUpgrader ModelUpgraderAPI,
	controllerModelConfigAPI ModelConfigAPI,
	options ...modelcmd.WrapOption,
) cmd.Command {
	command := &upgradeModelCommand{
		modelConfigAPI:           modelConfigAPI,
		modelUpgraderAPI:         modelUpgrader,
		controllerModelConfigAPI: controllerModelConfigAPI,
	}
	command.SetClientStore(store)
	return modelcmd.Wrap(command, options...)
}

type upgradeModelSuite struct {
	testing.IsolationSuite

	modelConfigAPI           *mocks.MockModelConfigAPI
	modelUpgrader            *mocks.MockModelUpgraderAPI
	controllerModelConfigAPI *mocks.MockModelConfigAPI
	store                    *mocks.MockClientStore
}

var _ = tc.Suite(&upgradeModelSuite{})

func (s *upgradeModelSuite) upgradeModelCommand(c *tc.C, isCAAS bool) (*gomock.Controller, cmd.Command) {
	ctrl := gomock.NewController(c)
	s.modelConfigAPI = mocks.NewMockModelConfigAPI(ctrl)
	s.modelUpgrader = mocks.NewMockModelUpgraderAPI(ctrl)
	s.controllerModelConfigAPI = mocks.NewMockModelConfigAPI(ctrl)
	s.store = mocks.NewMockClientStore(ctrl)

	s.modelConfigAPI.EXPECT().Close().AnyTimes()
	s.modelUpgrader.EXPECT().Close().AnyTimes()
	s.controllerModelConfigAPI.EXPECT().Close().AnyTimes()

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

	return ctrl, newUpgradeJujuCommandForTest(
		s.store,
		s.modelConfigAPI, s.modelUpgrader, s.controllerModelConfigAPI,
	)
}

func (s *upgradeModelSuite) TestUpgradeModelProvidedAgentVersionUpToDate(c *tc.C) {
	ctrl, cmd := s.upgradeModelCommand(c, false)
	defer ctrl.Finish()

	cfg := coretesting.FakeConfig().Merge(coretesting.Attrs{
		"agent-version": coretesting.FakeVersionNumber.String(),
	})

	s.modelConfigAPI.EXPECT().ModelGet(gomock.Any()).Return(cfg, nil)

	ctx, err := cmdtesting.RunCommand(c, cmd, "--agent-version", coretesting.FakeVersionNumber.String())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(ctx), tc.Equals, "no upgrades available\n")
}

func (s *upgradeModelSuite) TestUpgradeModelWithAgentVersion(c *tc.C) {
	ctrl, cmd := s.upgradeModelCommand(c, false)
	defer ctrl.Finish()

	cfg := coretesting.FakeConfig().Merge(coretesting.Attrs{
		// TODO (hml) 19-oct-2022
		// Once upgrade from 2.9 to 3.0 is supported, go back to
		// using coretesting.FakeVersionNumber.String() in this
		// test.
		//"agent-version": coretesting.FakeVersionNumber.String(),
		"agent-version": "3.0.1",
	})
	controllerCfg := coretesting.FakeConfig().Merge(coretesting.Attrs{
		"agent-version": coretesting.FakeVersionNumber.String(),
		"uuid":          testControllerModelUUID,
	})

	gomock.InOrder(
		s.modelConfigAPI.EXPECT().ModelGet(gomock.Any()).Return(cfg, nil),
		s.controllerModelConfigAPI.EXPECT().ModelGet(gomock.Any()).Return(controllerCfg, nil),
		s.modelUpgrader.EXPECT().UpgradeModel(
			gomock.Any(),
			coretesting.ModelTag.Id(), semversion.MustParse("3.9.99"),
			"", false, false,
		).Return(semversion.MustParse("3.9.99"), nil),
	)

	ctx, err := cmdtesting.RunCommand(c, cmd,
		"--agent-version", semversion.MustParse("3.9.99").String(),
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(ctx), tc.Equals, `
best version:
    3.9.99
`[1:])
	c.Assert(cmdtesting.Stdout(ctx), tc.Equals, `
started upgrade to 3.9.99
`[1:])
}

func (s *upgradeModelSuite) TestUpgradeModelFailsWithAgentVersionMissingButLocalOfficialAvailable(c *tc.C) {
	s.reset(c)

	s.PatchValue(&jujuversion.Current, func() semversion.Number {
		v := jujuversion.Current
		v.Build = 0
		return v
	}())

	ctrl, cmd := s.upgradeModelCommand(c, false)
	defer ctrl.Finish()

	agentVersion := coretesting.FakeVersionNumber
	cfg := coretesting.FakeConfig().Merge(coretesting.Attrs{
		"agent-version": agentVersion.String(),
	})
	controllerCfg := coretesting.FakeConfig().Merge(coretesting.Attrs{
		"agent-version": agentVersion.String(),
		"uuid":          testControllerModelUUID,
	})

	c.Assert(agentVersion.Build, tc.Equals, 0)
	builtVersion := coretesting.CurrentVersion()
	targetVersion := builtVersion.Number
	builtVersion.Build++
	gomock.InOrder(
		s.modelConfigAPI.EXPECT().ModelGet(gomock.Any()).Return(cfg, nil),
		s.controllerModelConfigAPI.EXPECT().ModelGet(gomock.Any()).Return(controllerCfg, nil),
		s.modelUpgrader.EXPECT().UpgradeModel(
			gomock.Any(),
			coretesting.ModelTag.Id(), targetVersion,
			"", false, false,
		).Return(
			semversion.Zero,
			errors.NotFoundf("available agent tool, upload required"),
		),
	)

	ctx, err := cmdtesting.RunCommand(c, cmd,
		"--agent-version", targetVersion.String(),
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(ctx), tc.Equals, `
no upgrades available
`[1:])
	c.Assert(cmdtesting.Stdout(ctx), tc.Equals, "")
}

func (s *upgradeModelSuite) TestUpgradeModelWithAgentVersionAlreadyUpToDate(c *tc.C) {
	s.reset(c)

	ctrl, cmd := s.upgradeModelCommand(c, false)
	defer ctrl.Finish()

	agentVersion := coretesting.FakeVersionNumber
	cfg := coretesting.FakeConfig().Merge(coretesting.Attrs{
		"agent-version": agentVersion.String(),
	})
	controllerCfg := coretesting.FakeConfig().Merge(coretesting.Attrs{
		"agent-version": agentVersion.String(),
		"uuid":          testControllerModelUUID,
	})

	c.Assert(agentVersion.Build, tc.Equals, 0)
	targetVersion := coretesting.CurrentVersion()
	gomock.InOrder(
		s.modelConfigAPI.EXPECT().ModelGet(gomock.Any()).Return(cfg, nil),
		s.controllerModelConfigAPI.EXPECT().ModelGet(gomock.Any()).Return(controllerCfg, nil),
		s.modelUpgrader.EXPECT().UpgradeModel(
			gomock.Any(),
			coretesting.ModelTag.Id(), targetVersion.ToPatch(),
			"", false, false,
		).Return(
			semversion.Zero,
			errors.AlreadyExistsf("up to date"),
		),
	)

	ctx, err := cmdtesting.RunCommand(c, cmd,
		"--agent-version", targetVersion.ToPatch().String(),
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(ctx), tc.Equals, "no upgrades available\n")
}

func (s *upgradeModelSuite) TestUpgradeModelWithAgentVersionFailedExpectUploadButWrongTargetVersion(c *tc.C) {
	s.reset(c)

	ctrl, cmd := s.upgradeModelCommand(c, false)
	defer ctrl.Finish()

	agentVersion := coretesting.FakeVersionNumber
	cfg := coretesting.FakeConfig().Merge(coretesting.Attrs{
		"agent-version": agentVersion.String(),
	})
	controllerCfg := coretesting.FakeConfig().Merge(coretesting.Attrs{
		"agent-version": agentVersion.String(),
		"uuid":          testControllerModelUUID,
	})

	current := agentVersion
	current.Minor++ // local snap is newer.
	s.PatchValue(&jujuversion.Current, current)

	targetVersion := current
	targetVersion.Patch++ // wrong target version (It has to be equal to local snap version).
	c.Assert(targetVersion.Compare(current) == 0, jc.IsFalse)

	gomock.InOrder(
		s.modelConfigAPI.EXPECT().ModelGet(gomock.Any()).Return(cfg, nil),
		s.controllerModelConfigAPI.EXPECT().ModelGet(gomock.Any()).Return(controllerCfg, nil),
		s.modelUpgrader.EXPECT().UpgradeModel(
			gomock.Any(),
			coretesting.ModelTag.Id(), targetVersion,
			"", false, false,
		).Return(
			semversion.Zero,
			errors.NotFoundf("available agent tool, upload required"),
		),
	)

	ctx, err := cmdtesting.RunCommand(c, cmd,
		"--agent-version", targetVersion.String(),
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(ctx), tc.Equals, "no upgrades available\n")
}

func (s *upgradeModelSuite) TestUpgradeModelWithAgentVersionExpectUploadFailedDueToNotAllowed(c *tc.C) {
	s.reset(c)

	s.PatchValue(&CheckCanImplicitUpload,
		func(model.ModelType, bool, semversion.Number, semversion.Number) bool { return false },
	)

	ctrl, cmd := s.upgradeModelCommand(c, false)
	defer ctrl.Finish()

	agentVersion := coretesting.FakeVersionNumber
	cfg := coretesting.FakeConfig().Merge(coretesting.Attrs{
		"agent-version": agentVersion.String(),
	})
	controllerCfg := coretesting.FakeConfig().Merge(coretesting.Attrs{
		"agent-version": agentVersion.String(),
		"uuid":          testControllerModelUUID,
	})

	targetVersion := coretesting.CurrentVersion().Number
	gomock.InOrder(
		s.modelConfigAPI.EXPECT().ModelGet(gomock.Any()).Return(cfg, nil),
		s.controllerModelConfigAPI.EXPECT().ModelGet(gomock.Any()).Return(controllerCfg, nil),
		s.modelUpgrader.EXPECT().UpgradeModel(
			gomock.Any(),
			coretesting.ModelTag.Id(), targetVersion,
			"", false, false,
		).Return(
			semversion.Zero,
			errors.NotFoundf("available agent tool, upload required"),
		),
	)

	ctx, err := cmdtesting.RunCommand(c, cmd,
		"--agent-version", targetVersion.String(),
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(ctx), tc.Equals, "no upgrades available\n")
}

func (s *upgradeModelSuite) TestUpgradeModelWithAgentVersionExpectUploadFailed(c *tc.C) {
	s.reset(c)

	ctrl, cmd := s.upgradeModelCommand(c, false)
	defer ctrl.Finish()

	agentVersion := coretesting.FakeVersionNumber
	modelCfg := coretesting.FakeConfig().Merge(coretesting.Attrs{
		"agent-version": agentVersion.String(),
	})
	controllerCfg := coretesting.FakeConfig().Merge(coretesting.Attrs{
		"agent-version": agentVersion.String(),
		"uuid":          testControllerModelUUID,
	})

	builtVersion := coretesting.CurrentVersion()
	targetVersion := builtVersion.Number
	gomock.InOrder(
		s.modelConfigAPI.EXPECT().ModelGet(gomock.Any()).Return(modelCfg, nil),
		s.controllerModelConfigAPI.EXPECT().ModelGet(gomock.Any()).Return(controllerCfg, nil),
		s.modelUpgrader.EXPECT().UpgradeModel(
			gomock.Any(),
			coretesting.ModelTag.Id(), targetVersion,
			"", false, false,
		).Return(
			semversion.Zero,
			errors.NotFoundf("available agent tool, upload required"),
		),
	)

	ctx, err := cmdtesting.RunCommand(c, cmd,
		"--agent-version", targetVersion.String(),
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(ctx), tc.Equals, "no upgrades available\n")
}

func (s *upgradeModelSuite) TestUpgradeModelWithAgentVersionDryRun(c *tc.C) {
	ctrl, cmd := s.upgradeModelCommand(c, false)
	defer ctrl.Finish()

	cfg := coretesting.FakeConfig().Merge(coretesting.Attrs{
		// TODO (hml) 19-oct-2022
		// Once upgrade from 2.9 to 3.0 is supported, go back to
		// using coretesting.FakeVersionNumber.String() in this
		// test.
		//"agent-version": coretesting.FakeVersionNumber.String(),
		"agent-version": "3.0.1",
	})
	controllerCfg := coretesting.FakeConfig().Merge(coretesting.Attrs{
		"agent-version": coretesting.FakeVersionNumber.String(),
		"uuid":          testControllerModelUUID,
	})

	gomock.InOrder(
		s.modelConfigAPI.EXPECT().ModelGet(gomock.Any()).Return(cfg, nil),
		s.controllerModelConfigAPI.EXPECT().ModelGet(gomock.Any()).Return(controllerCfg, nil),
		s.modelUpgrader.EXPECT().UpgradeModel(
			gomock.Any(),
			coretesting.ModelTag.Id(), semversion.MustParse("3.9.99"),
			"", false, true,
		).Return(semversion.MustParse("3.9.99"), nil),
	)

	ctx, err := cmdtesting.RunCommand(c, cmd,
		"--agent-version", semversion.MustParse("3.9.99").String(), "--dry-run",
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(ctx), tc.Equals, `
best version:
    3.9.99
upgrade to this version by running
    juju upgrade-model
`[1:])
}

func (s *upgradeModelSuite) TestUpgradeModelWithAgentVersionGotBlockers(c *tc.C) {
	ctrl, cmd := s.upgradeModelCommand(c, false)
	defer ctrl.Finish()

	cfg := coretesting.FakeConfig().Merge(coretesting.Attrs{
		// TODO (hml) 19-oct-2022
		// Once upgrade from 2.9 to 3.0 is supported, go back to
		// using coretesting.FakeVersionNumber.String() in this
		// test.
		//"agent-version": coretesting.FakeVersionNumber.String(),
		"agent-version": "3.0.1",
	})
	controllerCfg := coretesting.FakeConfig().Merge(coretesting.Attrs{
		"agent-version": coretesting.FakeVersionNumber.String(),
		"uuid":          testControllerModelUUID,
	})

	gomock.InOrder(
		s.modelConfigAPI.EXPECT().ModelGet(gomock.Any()).Return(cfg, nil),
		s.controllerModelConfigAPI.EXPECT().ModelGet(gomock.Any()).Return(controllerCfg, nil),
		s.modelUpgrader.EXPECT().UpgradeModel(
			gomock.Any(),
			coretesting.ModelTag.Id(), semversion.MustParse("3.9.99"),
			"", false, false,
		).Return(semversion.Zero, errors.New(`
cannot upgrade to "3.9.99" due to issues with these models:
"admin/default":
- the model hosts deprecated ubuntu machine(s): bionic(3) (not supported)
`[1:])),
	)

	_, err := cmdtesting.RunCommand(c, cmd,
		"--agent-version", semversion.MustParse("3.9.99").String(),
	)
	c.Assert(err.Error(), tc.Equals, `
cannot upgrade to "3.9.99" due to issues with these models:
"admin/default":
- the model hosts deprecated ubuntu machine(s): bionic(3) (not supported)
`[1:])
}

func (s *upgradeModelSuite) reset(c *tc.C) {
	s.PatchValue(&sync.BuildAgentTarball, toolstesting.GetMockBuildTools(c))
}

func (s *upgradeModelSuite) TestUpgradeModelUpToDate(c *tc.C) {
	s.reset(c)

	ctrl, cmd := s.upgradeModelCommand(c, false)
	defer ctrl.Finish()

	agentVersion := coretesting.FakeVersionNumber

	cfg := coretesting.FakeConfig().Merge(coretesting.Attrs{
		"agent-version": agentVersion.String(),
	})
	controllerCfg := coretesting.FakeConfig().Merge(coretesting.Attrs{
		"agent-version": agentVersion.String(),
		"uuid":          testControllerModelUUID,
	})

	gomock.InOrder(
		s.modelConfigAPI.EXPECT().ModelGet(gomock.Any()).Return(cfg, nil),
		s.controllerModelConfigAPI.EXPECT().ModelGet(gomock.Any()).Return(controllerCfg, nil),
		s.modelUpgrader.EXPECT().UpgradeModel(
			gomock.Any(),
			coretesting.ModelTag.Id(), semversion.Zero,
			"", false, false,
		).Return(semversion.Zero, errors.AlreadyExistsf("up to date")),
	)

	ctx, err := cmdtesting.RunCommand(c, cmd)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(ctx), tc.Equals, "no upgrades available\n")
}

func (s *upgradeModelSuite) TestUpgradeModelUpgradeToPublishedVersion(c *tc.C) {
	s.reset(c)

	ctrl, cmd := s.upgradeModelCommand(c, false)
	defer ctrl.Finish()

	agentVersion := coretesting.FakeVersionNumber
	cfg := coretesting.FakeConfig().Merge(coretesting.Attrs{
		"agent-version": agentVersion.String(),
	})
	controllerCfg := coretesting.FakeConfig().Merge(coretesting.Attrs{
		"agent-version": agentVersion.String(),
		"uuid":          testControllerModelUUID,
	})

	gomock.InOrder(
		s.modelConfigAPI.EXPECT().ModelGet(gomock.Any()).Return(cfg, nil),
		s.controllerModelConfigAPI.EXPECT().ModelGet(gomock.Any()).Return(controllerCfg, nil),
		s.modelUpgrader.EXPECT().UpgradeModel(
			gomock.Any(),
			coretesting.ModelTag.Id(), semversion.Zero,
			"", false, false,
		).Return(semversion.MustParse("3.9.99"), nil),
	)

	ctx, err := cmdtesting.RunCommand(c, cmd)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(ctx), tc.Equals, `
best version:
    3.9.99
`[1:])
	c.Assert(cmdtesting.Stdout(ctx), tc.Equals, `
started upgrade to 3.9.99
`[1:])
}

func (s *upgradeModelSuite) TestUpgradeModelWithStream(c *tc.C) {
	s.reset(c)

	ctrl, cmd := s.upgradeModelCommand(c, false)
	defer ctrl.Finish()

	agentVersion := coretesting.FakeVersionNumber
	cfg := coretesting.FakeConfig().Merge(coretesting.Attrs{
		"agent-version": agentVersion.String(),
	})
	controllerCfg := coretesting.FakeConfig().Merge(coretesting.Attrs{
		"agent-version": agentVersion.String(),
		"uuid":          testControllerModelUUID,
	})

	gomock.InOrder(
		s.modelConfigAPI.EXPECT().ModelGet(gomock.Any()).Return(cfg, nil),
		s.controllerModelConfigAPI.EXPECT().ModelGet(gomock.Any()).Return(controllerCfg, nil),
		s.modelUpgrader.EXPECT().UpgradeModel(
			gomock.Any(),
			coretesting.ModelTag.Id(), semversion.Zero,
			"proposed", false, false,
		).Return(semversion.MustParse("3.9.99"), nil),
	)

	ctx, err := cmdtesting.RunCommand(c, cmd, "--agent-stream", "proposed")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(ctx), tc.Equals, `
best version:
    3.9.99
`[1:])
	c.Assert(cmdtesting.Stdout(ctx), tc.Equals, `
started upgrade to 3.9.99
`[1:])
}

func (s *upgradeModelSuite) TestCheckCanImplicitUploadIAASModel(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	// Not IAAS model.
	canImplicitUpload := checkCanImplicitUpload(
		model.CAAS, true,
		semversion.MustParse("3.0.0"),
		semversion.MustParse("3.9.99.1"),
	)
	c.Check(canImplicitUpload, jc.IsFalse)

	// not official client.
	canImplicitUpload = checkCanImplicitUpload(
		model.IAAS, false,
		semversion.MustParse("3.9.99"),
		semversion.MustParse("3.0.0"),
	)
	c.Check(canImplicitUpload, jc.IsFalse)

	// non newer client.
	canImplicitUpload = checkCanImplicitUpload(
		model.IAAS, true,
		semversion.MustParse("2.9.99"),
		semversion.MustParse("3.0.0"),
	)
	c.Check(canImplicitUpload, jc.IsFalse)

	// client version with build number.
	canImplicitUpload = checkCanImplicitUpload(
		model.IAAS, true,
		semversion.MustParse("3.0.0.1"),
		semversion.MustParse("3.0.0"),
	)
	c.Check(canImplicitUpload, jc.IsTrue)

	// agent version with build number.
	canImplicitUpload = checkCanImplicitUpload(
		model.IAAS, true,
		semversion.MustParse("3.0.0"),
		semversion.MustParse("3.0.0.1"),
	)
	c.Check(canImplicitUpload, jc.IsTrue)

	// both client and agent version with build number == 0.
	canImplicitUpload = checkCanImplicitUpload(
		model.IAAS, true,
		semversion.MustParse("3.0.0"),
		semversion.MustParse("3.0.0"),
	)
	c.Check(canImplicitUpload, jc.IsFalse)
}
