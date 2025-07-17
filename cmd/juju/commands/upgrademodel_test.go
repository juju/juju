// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"bytes"

	"github.com/juju/cmd/v3"
	"github.com/juju/cmd/v3/cmdtesting"
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
	coretesting "github.com/juju/juju/testing"
	jujuversion "github.com/juju/juju/version"
)

const (
	testControllerModelUUID = "0badf00d-0bad-400d-8000-4b1d0d06f00d"
)

func newUpgradeJujuCommandForTest(
	store jujuclient.ClientStore,
	modelConfigAPI ModelConfigAPI,
	modelUpgrader ModelUpgraderAPI,
	controllerAPI ControllerAPI,
	options ...modelcmd.WrapOption,
) cmd.Command {
	command := &upgradeModelCommand{
		modelConfigAPI:   modelConfigAPI,
		modelUpgraderAPI: modelUpgrader,
		controllerAPI:    controllerAPI,
	}
	command.SetClientStore(store)
	return modelcmd.Wrap(command, options...)
}

type upgradeModelSuite struct {
	testing.IsolationSuite

	modelConfigAPI *mocks.MockModelConfigAPI
	modelUpgrader  *mocks.MockModelUpgraderAPI
	controllerAPI  *mocks.MockControllerAPI
	store          *mocks.MockClientStore
}

var _ = gc.Suite(&upgradeModelSuite{})

func (s *upgradeModelSuite) upgradeModelCommand(c *gc.C, isCAAS bool) (*gomock.Controller, cmd.Command) {
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

func (s *upgradeModelSuite) TestUpgradeModelProvidedAgentVersionUpToDate(c *gc.C) {
	ctrl, cmd := s.upgradeModelCommand(c, false)
	defer ctrl.Finish()

	cfg := coretesting.FakeConfig().Merge(coretesting.Attrs{
		"agent-version": coretesting.FakeVersionNumber.String(),
	})

	s.modelConfigAPI.EXPECT().ModelGet().Return(cfg, nil)

	ctx, err := cmdtesting.RunCommand(c, cmd, "--agent-version", coretesting.FakeVersionNumber.String())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(ctx), gc.Equals, "no upgrades available\n")
}

func (s *upgradeModelSuite) TestUpgradeModelWithAgentVersion(c *gc.C) {
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
		s.modelConfigAPI.EXPECT().ModelGet().Return(cfg, nil),
		s.controllerAPI.EXPECT().ModelConfig().Return(controllerCfg, nil),
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

func (s *upgradeModelSuite) TestUpgradeModelFailsWithAgentVersionMissingButLocalOfficialAvailable(c *gc.C) {
	s.reset(c)

	s.PatchValue(&jujuversion.Current, func() version.Number {
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

	c.Assert(agentVersion.Build, gc.Equals, 0)
	builtVersion := coretesting.CurrentVersion()
	targetVersion := builtVersion.Number
	builtVersion.Build++
	gomock.InOrder(
		s.modelConfigAPI.EXPECT().ModelGet().Return(cfg, nil),
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
	c.Assert(cmdtesting.Stderr(ctx), gc.Equals, `
no upgrades available
`[1:])
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, "")
}

func (s *upgradeModelSuite) TestUpgradeModelWithAgentVersionAlreadyUpToDate(c *gc.C) {
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

	c.Assert(agentVersion.Build, gc.Equals, 0)
	targetVersion := coretesting.CurrentVersion()
	gomock.InOrder(
		s.modelConfigAPI.EXPECT().ModelGet().Return(cfg, nil),
		s.controllerAPI.EXPECT().ModelConfig().Return(controllerCfg, nil),
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

func (s *upgradeModelSuite) TestUpgradeModelWithAgentVersionFailedExpectUploadButWrongTargetVersion(c *gc.C) {
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
		s.modelConfigAPI.EXPECT().ModelGet().Return(cfg, nil),
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

func (s *upgradeModelSuite) TestUpgradeModelWithAgentVersionExpectUploadFailedDueToNotAllowed(c *gc.C) {
	s.reset(c)

	s.PatchValue(&CheckCanImplicitUpload,
		func(model.ModelType, bool, version.Number, string, version.Number) bool { return false },
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
		s.modelConfigAPI.EXPECT().ModelGet().Return(cfg, nil),
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

func (s *upgradeModelSuite) TestUpgradeModelWithAgentVersionExpectUploadFailed(c *gc.C) {
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

func (s *upgradeModelSuite) TestUpgradeModelWithAgentVersionDryRun(c *gc.C) {
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
		s.modelConfigAPI.EXPECT().ModelGet().Return(cfg, nil),
		s.controllerAPI.EXPECT().ModelConfig().Return(controllerCfg, nil),
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

func (s *upgradeModelSuite) TestUpgradeModelWithAgentVersionGotBlockers(c *gc.C) {
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
		s.modelConfigAPI.EXPECT().ModelGet().Return(cfg, nil),
		s.controllerAPI.EXPECT().ModelConfig().Return(controllerCfg, nil),
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

func (s *upgradeModelSuite) reset(c *gc.C) {
	s.PatchValue(&sync.BuildAgentTarball, toolstesting.GetMockBuildTools(c))
}

func (s *upgradeModelSuite) TestUpgradeModelUpToDate(c *gc.C) {
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
		s.modelConfigAPI.EXPECT().ModelGet().Return(cfg, nil),
		s.controllerAPI.EXPECT().ModelConfig().Return(controllerCfg, nil),
		s.modelUpgrader.EXPECT().UpgradeModel(
			coretesting.ModelTag.Id(), version.Zero,
			"", false, false,
		).Return(version.Zero, errors.AlreadyExistsf("up to date")),
	)

	ctx, err := cmdtesting.RunCommand(c, cmd)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(ctx), gc.Equals, "no upgrades available\n")
}

func (s *upgradeModelSuite) TestUpgradeModelUpgradeToPublishedVersion(c *gc.C) {
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
		s.modelConfigAPI.EXPECT().ModelGet().Return(cfg, nil),
		s.controllerAPI.EXPECT().ModelConfig().Return(controllerCfg, nil),
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

func (s *upgradeModelSuite) TestUpgradeModelWithStream(c *gc.C) {
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
		s.modelConfigAPI.EXPECT().ModelGet().Return(cfg, nil),
		s.controllerAPI.EXPECT().ModelConfig().Return(controllerCfg, nil),
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

func (s *upgradeModelSuite) assertResetPreviousUpgrade(c *gc.C, answer string, expectUpgrade bool, args ...string) {
	s.reset(c)

	c.Logf("answer %q, expectUpgrade %v, args %s", answer, expectUpgrade, args)

	ctx := cmdtesting.Context(c)
	var stdin bytes.Buffer
	ctx.Stdin = &stdin

	if answer != "" {
		stdin.WriteString(answer)
	}

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

	assertions := []any{
		s.modelConfigAPI.EXPECT().ModelGet().Return(cfg, nil),
		s.controllerAPI.EXPECT().ModelConfig().Return(controllerCfg, nil),
	}
	if expectUpgrade {
		assertions = append(assertions,
			s.modelUpgrader.EXPECT().AbortModelUpgrade(coretesting.ModelTag.Id()).Return(nil),
			s.modelUpgrader.EXPECT().UpgradeModel(
				coretesting.ModelTag.Id(), version.Zero, "", false, false,
			).Return(version.MustParse("3.9.99"), nil),
		)
	}

	gomock.InOrder(assertions...)

	err := cmdtesting.InitCommand(cmd,
		append([]string{"--reset-previous-upgrade"}, args...))
	c.Assert(err, jc.ErrorIsNil)
	err = cmd.Run(ctx)
	if expectUpgrade {
		// ctx, err := cmdtesting.RunCommand(c, cmd)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(cmdtesting.Stderr(ctx), gc.Equals, `
best version:
    3.9.99
`[1:])
		if answer != "" {
			c.Assert(cmdtesting.Stdout(ctx), gc.Equals, `
WARNING! using --reset-previous-upgrade when an upgrade is in progress
will cause the upgrade to fail. Only use this option to clear an
incomplete upgrade where the root cause has been resolved.

Continue [y/N]? started upgrade to 3.9.99
`)
		} else {
			c.Assert(cmdtesting.Stdout(ctx), gc.Equals, `
started upgrade to 3.9.99
`[1:])
		}

	} else {
		c.Assert(err, gc.ErrorMatches, "previous upgrade not reset and no new upgrade triggered")
	}
}

func (s *upgradeModelSuite) TestResetPreviousUpgrade(c *gc.C) {
	s.assertResetPreviousUpgrade(c, "", false)

	s.assertResetPreviousUpgrade(c, "", true, "-y")
	s.assertResetPreviousUpgrade(c, "", true, "--yes")
	s.assertResetPreviousUpgrade(c, "y", true)
	s.assertResetPreviousUpgrade(c, "Y", true)
	s.assertResetPreviousUpgrade(c, "yes", true)
	s.assertResetPreviousUpgrade(c, "YES", true)

	s.assertResetPreviousUpgrade(c, "n", false)
	s.assertResetPreviousUpgrade(c, "N", false)
	s.assertResetPreviousUpgrade(c, "no", false)
	s.assertResetPreviousUpgrade(c, "foo", false)
}
