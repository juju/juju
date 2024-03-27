// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"bytes"
	"fmt"

	"github.com/juju/cmd/v3"
	"github.com/juju/cmd/v3/cmdtesting"
	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version/v2"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facades/client/client"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/cmd/juju/commands/mocks"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/environs/sync"
	toolstesting "github.com/juju/juju/environs/tools/testing"
	"github.com/juju/juju/internal/devtools"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/jujuclient"
	coretesting "github.com/juju/juju/testing"
	jujuversion "github.com/juju/juju/version"
)

type OldUpgradeControllerSuite struct {
	jujutesting.JujuConnSuite

	resources  *common.Resources
	authoriser apiservertesting.FakeAuthorizer
	coretesting.CmdBlockHelper
	modelUpgrader *mocks.MockModelUpgraderAPI
}

var _ = gc.Suite(&OldUpgradeControllerSuite{})

func (s *OldUpgradeControllerSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	client.SkipReplicaCheck(s)

	s.resources = common.NewResources()
	s.authoriser = apiservertesting.FakeAuthorizer{
		Tag: s.AdminUserTag(c),
	}

	s.CmdBlockHelper = coretesting.NewCmdBlockHelper(s.APIState)
	c.Assert(s.CmdBlockHelper, gc.NotNil)
	s.AddCleanup(func(*gc.C) { s.CmdBlockHelper.Close() })
}

func (s *OldUpgradeControllerSuite) upgradeControllerCommand(c *gc.C) (*gomock.Controller, cmd.Command) {
	ctrl := gomock.NewController(c)
	s.modelUpgrader = mocks.NewMockModelUpgraderAPI(ctrl)
	cmd := &upgradeControllerCommand{
		modelUpgraderAPI: s.modelUpgrader,
	}
	cmd.SetClientStore(s.ControllerStore)
	return ctrl, modelcmd.WrapController(cmd)
}

func (s *OldUpgradeControllerSuite) TestUpgradeWrongPermissions(c *gc.C) {
	details, err := s.ControllerStore.AccountDetails("kontroll")
	c.Assert(err, jc.ErrorIsNil)
	details.LastKnownAccess = string(permission.ReadAccess)
	err = s.ControllerStore.UpdateAccount("kontroll", *details)
	c.Assert(err, jc.ErrorIsNil)
	_, com := s.upgradeControllerCommand(c)
	err = cmdtesting.InitCommand(com, []string{})
	c.Assert(err, jc.ErrorIsNil)
	ctx := cmdtesting.Context(c)
	err = com.Run(ctx)
	expectedErrMsg := fmt.Sprintf("upgrade not possible missing"+
		" permissions, current level %q, need: %q", details.LastKnownAccess, permission.SuperuserAccess)
	c.Assert(err, gc.ErrorMatches, expectedErrMsg)
}

func (s *OldUpgradeControllerSuite) TestUpgradeDifferentUser(c *gc.C) {
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

	cmd := &upgradeControllerCommand{}
	cmd.SetClientStore(s.ControllerStore)
	cmdrun := modelcmd.WrapController(cmd)
	_, err = cmdtesting.RunCommand(c, cmdrun)
	c.Assert(err, jc.ErrorIsNil)
}

func newUpgradeControllerCommandForTest(
	store jujuclient.ClientStore,
	modelConfigAPI ModelConfigAPI,
	modelUpgrader ModelUpgraderAPI,
	controllerAPI ControllerAPI,
	options ...modelcmd.WrapControllerOption,
) cmd.Command {
	command := &upgradeControllerCommand{
		modelConfigAPI:   modelConfigAPI,
		modelUpgraderAPI: modelUpgrader,
		controllerAPI:    controllerAPI,
	}
	command.SetClientStore(store)
	return modelcmd.WrapController(command, options...)
}

type upgradeControllerSuite struct {
	testing.IsolationSuite

	modelConfigAPI *mocks.MockModelConfigAPI
	modelUpgrader  *mocks.MockModelUpgraderAPI
	controllerAPI  *mocks.MockControllerAPI
	store          *mocks.MockClientStore
}

var _ = gc.Suite(&upgradeControllerSuite{})

func (s *upgradeControllerSuite) upgradeControllerCommand(c *gc.C, isCAAS bool) (*gomock.Controller, cmd.Command) {
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
		s.modelConfigAPI, s.modelUpgrader, s.controllerAPI,
	)
}

func (s *upgradeControllerSuite) TestUpgradeControllerProvidedAgentVersionUpToDate(c *gc.C) {
	ctrl, cmd := s.upgradeControllerCommand(c, false)
	defer ctrl.Finish()

	cfg := coretesting.FakeConfig().Merge(coretesting.Attrs{
		"agent-version": coretesting.FakeVersionNumber.String(),
	})

	s.modelConfigAPI.EXPECT().ModelGet().Return(cfg, nil)

	ctx, err := cmdtesting.RunCommand(c, cmd, "--agent-version", coretesting.FakeVersionNumber.String())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(ctx), gc.Equals, "no upgrades available\n")
}

func (s *upgradeControllerSuite) TestUpgradeControllerFailedWithDevAndAgentVersion(c *gc.C) {
	ctrl, cmd := s.upgradeControllerCommand(c, false)
	defer ctrl.Finish()

	currentVersion := jujuversion.Current
	currentVersion.Build = 1
	s.PatchValue(&jujuversion.Current, currentVersion)
	s.PatchValue(&devtools.SourceDir, func() (string, error) {
		return "/dev/null", nil
	})

	_, err := cmdtesting.RunCommand(c, cmd,
		"--dev",
		"--agent-version", version.MustParse("3.9.99").String(),
	)
	c.Assert(err, gc.ErrorMatches, `--agent-version and --dev can't be used together`)
}

func (s *upgradeControllerSuite) TestUpgradeControllerWithAgentVersion(c *gc.C) {
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
		s.modelConfigAPI.EXPECT().ModelGet().Return(cfg, nil),
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

func (s *upgradeControllerSuite) TestUpgradeControllerWithAgentVersionNotFound(c *gc.C) {
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
		s.modelConfigAPI.EXPECT().ModelGet().Return(cfg, nil),
		s.modelUpgrader.EXPECT().UpgradeModel(
			coretesting.ModelTag.Id(), version.MustParse("3.9.99"),
			"", false, false,
		).Return(version.Zero, errors.NotFound),
	)

	_, err := cmdtesting.RunCommand(c, cmd,
		"--agent-version", version.MustParse("3.9.99").String(),
	)
	c.Assert(err, gc.ErrorMatches, `explicit agent version not available - try juju sync-agent-binary first`)
}

func (s *upgradeControllerSuite) TestUpgradeControllerWithAgentVersionAlreadyUpToDate(c *gc.C) {
	s.reset(c)

	ctrl, cmd := s.upgradeControllerCommand(c, false)
	defer ctrl.Finish()

	agentVersion := coretesting.FakeVersionNumber
	cfg := coretesting.FakeConfig().Merge(coretesting.Attrs{
		"agent-version": agentVersion.String(),
	})

	c.Assert(agentVersion.Build, gc.Equals, 0)
	targetVersion := coretesting.CurrentVersion()
	gomock.InOrder(
		s.modelConfigAPI.EXPECT().ModelGet().Return(cfg, nil),
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

func (s *upgradeControllerSuite) TestUpgradeControllerWithAgentVersionFailedExpectUploadButWrongTargetVersion(c *gc.C) {
	s.reset(c)

	//s.PatchValue(&CheckCanImplicitUpload,
	//	func(model.ModelType, bool, version.Number, version.Number) bool { return true },
	//)

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
	c.Assert(targetVersion.Compare(current) == 0, jc.IsFalse)

	gomock.InOrder(
		s.modelConfigAPI.EXPECT().ModelGet().Return(cfg, nil),
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

func (s *upgradeControllerSuite) TestUpgradeControllerWithAgentVersionExpectUploadFailedDueToNotAllowed(c *gc.C) {
	s.reset(c)

	//s.PatchValue(&CheckCanImplicitUpload,
	//	func(model.ModelType, bool, version.Number, version.Number) bool { return false },
	//)

	ctrl, cmd := s.upgradeControllerCommand(c, false)
	defer ctrl.Finish()

	agentVersion := coretesting.FakeVersionNumber
	cfg := coretesting.FakeConfig().Merge(coretesting.Attrs{
		"agent-version": agentVersion.String(),
	})

	targetVersion := coretesting.CurrentVersion().Number
	gomock.InOrder(
		s.modelConfigAPI.EXPECT().ModelGet().Return(cfg, nil),
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

func (s *upgradeControllerSuite) TestUpgradeControllerWithAgentVersionDryRun(c *gc.C) {
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
		s.modelConfigAPI.EXPECT().ModelGet().Return(cfg, nil),
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
    juju upgrade-controller
`[1:])
}

func (s *upgradeControllerSuite) TestUpgradeControllerWithAgentVersionGotBlockers(c *gc.C) {
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
		s.modelConfigAPI.EXPECT().ModelGet().Return(cfg, nil),
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

func (s *upgradeControllerSuite) reset(c *gc.C) {
	s.PatchValue(&sync.BuildAgentTarball, toolstesting.GetMockBuildTools(c, nil))
}

func (s *upgradeControllerSuite) TestUpgradeControllerWithDev(c *gc.C) {
	s.reset(c)

	ctrl, cmd := s.upgradeControllerCommand(c, false)
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
		s.modelUpgrader.EXPECT().UploadTools(gomock.Any(), builtVersion).Return(nil, nil),
		s.modelUpgrader.EXPECT().UpgradeModel(
			coretesting.ModelTag.Id(), builtVersion.Number,
			"", false, false,
		).Return(builtVersion.Number, nil),
	)

	ctx, err := cmdtesting.RunCommand(c, cmd, "--dev")
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

func (s *upgradeControllerSuite) TestUpgradeControllerUpToDate(c *gc.C) {
	s.reset(c)

	ctrl, cmd := s.upgradeControllerCommand(c, false)
	defer ctrl.Finish()

	agentVersion := coretesting.FakeVersionNumber
	cfg := coretesting.FakeConfig().Merge(coretesting.Attrs{
		"agent-version": agentVersion.String(),
	})

	gomock.InOrder(
		s.modelConfigAPI.EXPECT().ModelGet().Return(cfg, nil),
		s.modelUpgrader.EXPECT().UpgradeModel(
			coretesting.ModelTag.Id(), version.Zero,
			"", false, false,
		).Return(version.Zero, errors.AlreadyExistsf("up to date")),
	)

	ctx, err := cmdtesting.RunCommand(c, cmd)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(ctx), gc.Equals, "no upgrades available\n")
}

func (s *upgradeControllerSuite) TestUpgradeControllerUpgradeToPublishedVersion(c *gc.C) {
	s.reset(c)

	ctrl, cmd := s.upgradeControllerCommand(c, false)
	defer ctrl.Finish()

	agentVersion := coretesting.FakeVersionNumber
	cfg := coretesting.FakeConfig().Merge(coretesting.Attrs{
		"agent-version": agentVersion.String(),
	})

	gomock.InOrder(
		s.modelConfigAPI.EXPECT().ModelGet().Return(cfg, nil),
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

func (s *upgradeControllerSuite) TestUpgradeControllerWithStream(c *gc.C) {
	s.reset(c)

	ctrl, cmd := s.upgradeControllerCommand(c, false)
	defer ctrl.Finish()

	agentVersion := coretesting.FakeVersionNumber
	cfg := coretesting.FakeConfig().Merge(coretesting.Attrs{
		"agent-version": agentVersion.String(),
	})

	gomock.InOrder(
		s.modelConfigAPI.EXPECT().ModelGet().Return(cfg, nil),
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

func (s *upgradeControllerSuite) assertResetPreviousUpgrade(c *gc.C, answer string, expectUpgrade bool, args ...string) {
	s.reset(c)

	c.Logf("answer %q, expectUpgrade %v, args %s", answer, expectUpgrade, args)

	ctx := cmdtesting.Context(c)
	var stdin bytes.Buffer
	ctx.Stdin = &stdin

	if answer != "" {
		stdin.WriteString(answer)
	}

	ctrl, cmd := s.upgradeControllerCommand(c, false)
	defer ctrl.Finish()

	agentVersion := coretesting.FakeVersionNumber
	cfg := coretesting.FakeConfig().Merge(coretesting.Attrs{
		"agent-version": agentVersion.String(),
	})

	assertions := []any{
		s.modelConfigAPI.EXPECT().ModelGet().Return(cfg, nil),
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

func (s *upgradeControllerSuite) TestResetPreviousUpgrade(c *gc.C) {
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
