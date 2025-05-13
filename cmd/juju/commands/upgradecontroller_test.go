// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/tc"
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
	"github.com/juju/juju/internal/testhelpers"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/jujuclient"
)

func newUpgradeControllerCommandForTest(
	store jujuclient.ClientStore,
	modelConfigAPI ModelConfigAPI,
	modelUpgrader ModelUpgraderAPI,
	options ...modelcmd.WrapControllerOption,
) cmd.Command {
	command := &upgradeControllerCommand{
		modelConfigAPI:   modelConfigAPI,
		modelUpgraderAPI: modelUpgrader,
	}
	command.SetClientStore(store)
	return modelcmd.WrapController(command, options...)
}

type upgradeControllerSuite struct {
	testhelpers.IsolationSuite

	modelConfigAPI *mocks.MockModelConfigAPI
	modelUpgrader  *mocks.MockModelUpgraderAPI
	store          *mocks.MockClientStore
}

var _ = tc.Suite(&upgradeControllerSuite{})

func (s *upgradeControllerSuite) upgradeControllerCommand(c *tc.C, isCAAS bool) (*gomock.Controller, cmd.Command) {
	ctrl := gomock.NewController(c)
	s.modelConfigAPI = mocks.NewMockModelConfigAPI(ctrl)
	s.modelUpgrader = mocks.NewMockModelUpgraderAPI(ctrl)
	s.store = mocks.NewMockClientStore(ctrl)

	s.modelConfigAPI.EXPECT().Close().AnyTimes()
	s.modelUpgrader.EXPECT().Close().AnyTimes()

	s.store.EXPECT().CurrentController().AnyTimes().Return("c-1", nil)
	s.store.EXPECT().ControllerByName("c-1").AnyTimes().Return(&jujuclient.ControllerDetails{
		APIEndpoints: []string{"0.1.2.3:1234"},
	}, nil)
	s.store.EXPECT().CurrentModel("c-1").AnyTimes().Return("m-1", nil)
	s.store.EXPECT().AccountDetails("c-1").AnyTimes().Return(&jujuclient.AccountDetails{User: "foo", LastKnownAccess: "superuser"}, nil)
	cookieJar := mocks.NewMockCookieJar(ctrl)
	cookieJar.EXPECT().Save().AnyTimes().Return(nil)
	s.store.EXPECT().CookieJar("c-1").AnyTimes().Return(cookieJar, nil)

	modelType := model.IAAS
	if isCAAS {
		modelType = model.CAAS
	}

	s.store.EXPECT().ModelByName("c-1", "admin/controller").AnyTimes().Return(&jujuclient.ModelDetails{
		ModelUUID: coretesting.ModelTag.Id(),
		ModelType: modelType,
	}, nil)

	return ctrl, newUpgradeControllerCommandForTest(s.store,
		s.modelConfigAPI, s.modelUpgrader,
	)
}

func (s *upgradeControllerSuite) TestUpgradeModelFailedCAASWithBuildAgent(c *tc.C) {
	ctrl, cmd := s.upgradeControllerCommand(c, true)
	defer ctrl.Finish()

	_, err := cmdtesting.RunCommand(c, cmd, `--build-agent`)
	c.Assert(err, tc.ErrorMatches, `--build-agent for k8s model upgrades not supported`)
}

func (s *upgradeControllerSuite) TestUpgradeModelProvidedAgentVersionUpToDate(c *tc.C) {
	ctrl, cmd := s.upgradeControllerCommand(c, false)
	defer ctrl.Finish()

	cfg := coretesting.FakeConfig().Merge(coretesting.Attrs{
		"agent-version": coretesting.FakeVersionNumber.String(),
	})

	s.modelConfigAPI.EXPECT().ModelGet(gomock.Any()).Return(cfg, nil)

	ctx, err := cmdtesting.RunCommand(c, cmd, "--agent-version", coretesting.FakeVersionNumber.String())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(ctx), tc.Equals, "no upgrades available\n")
}

func (s *upgradeControllerSuite) TestUpgradeModelFailedWithBuildAgentAndAgentVersion(c *tc.C) {
	ctrl, cmd := s.upgradeControllerCommand(c, false)
	defer ctrl.Finish()

	cfg := coretesting.FakeConfig().Merge(coretesting.Attrs{
		"agent-version": coretesting.FakeVersionNumber.String(),
	})

	gomock.InOrder(
		s.modelConfigAPI.EXPECT().ModelGet(gomock.Any()).Return(cfg, nil),
	)

	_, err := cmdtesting.RunCommand(c, cmd,
		"--build-agent",
		"--agent-version", semversion.MustParse("3.9.99").String(),
	)
	c.Assert(err, tc.ErrorMatches, `--build-agent cannot be used with --agent-version together`)
}

func (s *upgradeControllerSuite) TestUpgradeModelWithAgentVersion(c *tc.C) {
	ctrl, cmd := s.upgradeControllerCommand(c, false)
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
		s.modelConfigAPI.EXPECT().ModelGet(gomock.Any()).Return(cfg, nil),
		s.modelUpgrader.EXPECT().UpgradeModel(
			gomock.Any(),
			coretesting.ModelTag.Id(), semversion.MustParse("3.9.99"),
			"", false, false,
		).Return(semversion.MustParse("3.9.99"), nil),
	)

	ctx, err := cmdtesting.RunCommand(c, cmd,
		"--agent-version", semversion.MustParse("3.9.99").String(),
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(ctx), tc.Equals, `
best version:
    3.9.99
`[1:])
	c.Assert(cmdtesting.Stdout(ctx), tc.Equals, `
started upgrade to 3.9.99
`[1:])
}

func (s *upgradeControllerSuite) TestUpgradeModelWithAgentVersionUploadLocalOfficial(c *tc.C) {
	s.reset(c)

	s.PatchValue(&jujuversion.Current, func() semversion.Number {
		v := jujuversion.Current
		v.Build = 0
		return v
	}())

	s.PatchValue(&CheckCanImplicitUpload,
		func(model.ModelType, bool, semversion.Number, semversion.Number) bool { return true },
	)

	ctrl, cmd := s.upgradeControllerCommand(c, false)
	defer ctrl.Finish()

	agentVersion := coretesting.FakeVersionNumber
	cfg := coretesting.FakeConfig().Merge(coretesting.Attrs{
		"agent-version": agentVersion.String(),
	})

	c.Assert(agentVersion.Build, tc.Equals, 0)
	builtVersion := coretesting.CurrentVersion()
	targetVersion := builtVersion.Number
	builtVersion.Build++
	gomock.InOrder(
		s.modelConfigAPI.EXPECT().ModelGet(gomock.Any()).Return(cfg, nil),
		s.modelUpgrader.EXPECT().UpgradeModel(
			gomock.Any(),
			coretesting.ModelTag.Id(), targetVersion,
			"", false, false,
		).Return(
			semversion.Zero,
			errors.NotFoundf("available agent tool, upload required"),
		),
		s.modelUpgrader.EXPECT().UploadTools(gomock.Any(), gomock.Any(), builtVersion).Return(nil, nil),
		s.modelUpgrader.EXPECT().UpgradeModel(
			gomock.Any(),
			coretesting.ModelTag.Id(), builtVersion.Number,
			"", false, false,
		).Return(builtVersion.Number, nil),
	)

	ctx, err := cmdtesting.RunCommand(c, cmd,
		"--agent-version", targetVersion.String(),
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(ctx), tc.Equals, fmt.Sprintf(`
best version:
    %s
`, builtVersion.Number)[1:])
	c.Assert(cmdtesting.Stdout(ctx), tc.Equals, fmt.Sprintf(`
no prepackaged agent binaries available, using the local snap jujud %s
started upgrade to %s
`, builtVersion.Number, builtVersion.Number)[1:])
}

func (s *upgradeControllerSuite) TestUpgradeModelWithAgentVersionAlreadyUpToDate(c *tc.C) {
	s.reset(c)

	ctrl, cmd := s.upgradeControllerCommand(c, false)
	defer ctrl.Finish()

	agentVersion := coretesting.FakeVersionNumber
	cfg := coretesting.FakeConfig().Merge(coretesting.Attrs{
		"agent-version": agentVersion.String(),
	})

	c.Assert(agentVersion.Build, tc.Equals, 0)
	targetVersion := coretesting.CurrentVersion()
	gomock.InOrder(
		s.modelConfigAPI.EXPECT().ModelGet(gomock.Any()).Return(cfg, nil),
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
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(ctx), tc.Equals, "no upgrades available\n")
}

func (s *upgradeControllerSuite) TestUpgradeModelWithAgentVersionFailedExpectUploadButWrongTargetVersion(c *tc.C) {
	s.reset(c)

	s.PatchValue(&CheckCanImplicitUpload,
		func(model.ModelType, bool, semversion.Number, semversion.Number) bool { return true },
	)

	ctrl, cmd := s.upgradeControllerCommand(c, false)
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
	c.Assert(targetVersion.Compare(current) == 0, tc.IsFalse)

	gomock.InOrder(
		s.modelConfigAPI.EXPECT().ModelGet(gomock.Any()).Return(cfg, nil),
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
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(ctx), tc.Equals, "no upgrades available\n")
}

func (s *upgradeControllerSuite) TestUpgradeModelWithAgentVersionExpectUploadFailedDueToNotAllowed(c *tc.C) {
	s.reset(c)

	s.PatchValue(&CheckCanImplicitUpload,
		func(model.ModelType, bool, semversion.Number, semversion.Number) bool { return false },
	)

	ctrl, cmd := s.upgradeControllerCommand(c, false)
	defer ctrl.Finish()

	agentVersion := coretesting.FakeVersionNumber
	cfg := coretesting.FakeConfig().Merge(coretesting.Attrs{
		"agent-version": agentVersion.String(),
	})

	targetVersion := coretesting.CurrentVersion().Number
	gomock.InOrder(
		s.modelConfigAPI.EXPECT().ModelGet(gomock.Any()).Return(cfg, nil),
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
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(ctx), tc.Equals, "no upgrades available\n")
}

func (s *upgradeControllerSuite) TestUpgradeModelWithAgentVersionDryRun(c *tc.C) {
	ctrl, cmd := s.upgradeControllerCommand(c, false)
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
		s.modelConfigAPI.EXPECT().ModelGet(gomock.Any()).Return(cfg, nil),
		s.modelUpgrader.EXPECT().UpgradeModel(
			gomock.Any(),
			coretesting.ModelTag.Id(), semversion.MustParse("3.9.99"),
			"", false, true,
		).Return(semversion.MustParse("3.9.99"), nil),
	)

	ctx, err := cmdtesting.RunCommand(c, cmd,
		"--agent-version", semversion.MustParse("3.9.99").String(), "--dry-run",
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(ctx), tc.Equals, `
best version:
    3.9.99
upgrade to this version by running
    juju upgrade-controller
`[1:])
}

func (s *upgradeControllerSuite) TestUpgradeModelWithAgentVersionGotBlockers(c *tc.C) {
	ctrl, cmd := s.upgradeControllerCommand(c, false)
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
		s.modelConfigAPI.EXPECT().ModelGet(gomock.Any()).Return(cfg, nil),
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

func (s *upgradeControllerSuite) reset(c *tc.C) {
	s.PatchValue(&sync.BuildAgentTarball, toolstesting.GetMockBuildTools(c))
}

func (s *upgradeControllerSuite) TestUpgradeModelWithBuildAgent(c *tc.C) {
	s.reset(c)

	ctrl, cmd := s.upgradeControllerCommand(c, false)
	defer ctrl.Finish()

	agentVersion := coretesting.FakeVersionNumber
	cfg := coretesting.FakeConfig().Merge(coretesting.Attrs{
		"agent-version": agentVersion.String(),
	})
	c.Assert(agentVersion.Build, tc.Equals, 0)
	builtVersion := coretesting.CurrentVersion()
	builtVersion.Build++
	gomock.InOrder(
		s.modelConfigAPI.EXPECT().ModelGet(gomock.Any()).Return(cfg, nil),
		s.modelUpgrader.EXPECT().UploadTools(gomock.Any(), gomock.Any(), builtVersion).Return(nil, nil),
		s.modelUpgrader.EXPECT().UpgradeModel(
			gomock.Any(),
			coretesting.ModelTag.Id(), builtVersion.Number,
			"", false, false,
		).Return(builtVersion.Number, nil),
	)

	ctx, err := cmdtesting.RunCommand(c, cmd, "--build-agent")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(ctx), tc.Equals, fmt.Sprintf(`
best version:
    %s
`, builtVersion.Number)[1:])
	c.Assert(cmdtesting.Stdout(ctx), tc.Equals, fmt.Sprintf(`
no prepackaged agent binaries available, using local agent binary %s (built from source)
started upgrade to %s
`, builtVersion.Number, builtVersion.Number)[1:])
}

func (s *upgradeControllerSuite) TestUpgradeModelUpToDate(c *tc.C) {
	s.reset(c)

	ctrl, cmd := s.upgradeControllerCommand(c, false)
	defer ctrl.Finish()

	agentVersion := coretesting.FakeVersionNumber
	cfg := coretesting.FakeConfig().Merge(coretesting.Attrs{
		"agent-version": agentVersion.String(),
	})

	gomock.InOrder(
		s.modelConfigAPI.EXPECT().ModelGet(gomock.Any()).Return(cfg, nil),
		s.modelUpgrader.EXPECT().UpgradeModel(
			gomock.Any(),
			coretesting.ModelTag.Id(), semversion.Zero,
			"", false, false,
		).Return(semversion.Zero, errors.AlreadyExistsf("up to date")),
	)

	ctx, err := cmdtesting.RunCommand(c, cmd)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(ctx), tc.Equals, "no upgrades available\n")
}

func (s *upgradeControllerSuite) TestUpgradeModelUpgradeToPublishedVersion(c *tc.C) {
	s.reset(c)

	ctrl, cmd := s.upgradeControllerCommand(c, false)
	defer ctrl.Finish()

	agentVersion := coretesting.FakeVersionNumber
	cfg := coretesting.FakeConfig().Merge(coretesting.Attrs{
		"agent-version": agentVersion.String(),
	})

	gomock.InOrder(
		s.modelConfigAPI.EXPECT().ModelGet(gomock.Any()).Return(cfg, nil),
		s.modelUpgrader.EXPECT().UpgradeModel(
			gomock.Any(),
			coretesting.ModelTag.Id(), semversion.Zero,
			"", false, false,
		).Return(semversion.MustParse("3.9.99"), nil),
	)

	ctx, err := cmdtesting.RunCommand(c, cmd)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(ctx), tc.Equals, `
best version:
    3.9.99
`[1:])
	c.Assert(cmdtesting.Stdout(ctx), tc.Equals, `
started upgrade to 3.9.99
`[1:])
}

func (s *upgradeControllerSuite) TestUpgradeModelWithStream(c *tc.C) {
	s.reset(c)

	ctrl, cmd := s.upgradeControllerCommand(c, false)
	defer ctrl.Finish()

	agentVersion := coretesting.FakeVersionNumber
	cfg := coretesting.FakeConfig().Merge(coretesting.Attrs{
		"agent-version": agentVersion.String(),
	})

	gomock.InOrder(
		s.modelConfigAPI.EXPECT().ModelGet(gomock.Any()).Return(cfg, nil),
		s.modelUpgrader.EXPECT().UpgradeModel(
			gomock.Any(),
			coretesting.ModelTag.Id(), semversion.Zero,
			"proposed", false, false,
		).Return(semversion.MustParse("3.9.99"), nil),
	)

	ctx, err := cmdtesting.RunCommand(c, cmd, "--agent-stream", "proposed")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(ctx), tc.Equals, `
best version:
    3.9.99
`[1:])
	c.Assert(cmdtesting.Stdout(ctx), tc.Equals, `
started upgrade to 3.9.99
`[1:])
}

func (s *upgradeControllerSuite) TestCheckCanImplicitUploadIAASModel(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	// Not IAAS model.
	canImplicitUpload := checkCanImplicitUpload(
		model.CAAS, true,
		semversion.MustParse("3.0.0"),
		semversion.MustParse("3.9.99.1"),
	)
	c.Check(canImplicitUpload, tc.IsFalse)

	// not official client.
	canImplicitUpload = checkCanImplicitUpload(
		model.IAAS, false,
		semversion.MustParse("3.9.99"),
		semversion.MustParse("3.0.0"),
	)
	c.Check(canImplicitUpload, tc.IsFalse)

	// non newer client.
	canImplicitUpload = checkCanImplicitUpload(
		model.IAAS, true,
		semversion.MustParse("2.9.99"),
		semversion.MustParse("3.0.0"),
	)
	c.Check(canImplicitUpload, tc.IsFalse)

	// client version with build number.
	canImplicitUpload = checkCanImplicitUpload(
		model.IAAS, true,
		semversion.MustParse("3.0.0.1"),
		semversion.MustParse("3.0.0"),
	)
	c.Check(canImplicitUpload, tc.IsTrue)

	// agent version with build number.
	canImplicitUpload = checkCanImplicitUpload(
		model.IAAS, true,
		semversion.MustParse("3.0.0"),
		semversion.MustParse("3.0.0.1"),
	)
	c.Check(canImplicitUpload, tc.IsTrue)

	// both client and agent version with build number == 0.
	canImplicitUpload = checkCanImplicitUpload(
		model.IAAS, true,
		semversion.MustParse("3.0.0"),
		semversion.MustParse("3.0.0"),
	)
	c.Check(canImplicitUpload, tc.IsFalse)
}
