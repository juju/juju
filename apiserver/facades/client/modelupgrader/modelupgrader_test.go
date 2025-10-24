// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelupgrader

import (
	"testing"

	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/apiserver/authentication"
	"github.com/juju/juju/apiserver/facade/mocks"
	modelupgradermocks "github.com/juju/juju/apiserver/facades/client/modelupgrader/mocks"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/domain/modelagent"
	modelagenterrors "github.com/juju/juju/domain/modelagent/errors"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/uuid"
	"github.com/juju/juju/rpc/params"
)

type modelUpgradeSuite struct {
	authorizer        *mocks.MockAuthorizer
	check             *modelupgradermocks.MockBlockCheckerInterface
	modelAgentService *modelupgradermocks.MockModelAgentService
	controllerTag     names.Tag
	modelTag          names.Tag
}

func TestModelUpgradeSuite(t *testing.T) {
	tc.Run(t, &modelUpgradeSuite{})
}

func (u *modelUpgradeSuite) setup(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	u.authorizer = mocks.NewMockAuthorizer(ctrl)
	u.check = modelupgradermocks.NewMockBlockCheckerInterface(ctrl)
	u.modelAgentService = modelupgradermocks.NewMockModelAgentService(ctrl)
	u.controllerTag = names.NewControllerTag(tc.Must(c, uuid.NewUUID).String())
	u.modelTag = names.NewModelTag(tc.Must(c, uuid.NewUUID).String())

	c.Cleanup(func() {
		u.authorizer = nil
		u.check = nil
		u.modelAgentService = nil
		u.controllerTag = nil
		u.modelTag = nil
	})
	return ctrl
}

// TestUpgradeModelWithVersionAndStream tests the upgrade with
// an explicit version and stream. This is a happy case.
func (u *modelUpgradeSuite) TestUpgradeModelWithVersionAndStream(c *tc.C) {
	ctrl := u.setup(c)
	defer ctrl.Finish()

	version, err := semversion.Parse("4.0.1")
	c.Assert(err, tc.ErrorIsNil)

	u.authorizer.EXPECT().HasPermission(
		gomock.Any(),
		permission.SuperuserAccess,
		u.controllerTag,
	).Return(nil)
	u.check.EXPECT().ChangeAllowed(gomock.Any()).Return(nil)

	u.modelAgentService.EXPECT().UpgradeModelTargetAgentVersionStreamTo(gomock.Any(), version, modelagent.AgentStreamReleased).Return(nil)

	api := NewModelUpgraderAPI(
		u.controllerTag,
		u.modelTag,
		u.authorizer, u.check, u.modelAgentService,
	)

	res, err := api.UpgradeModel(c.Context(), params.UpgradeModelParams{
		ModelTag:      u.modelTag.String(),
		TargetVersion: version,
		AgentStream:   "released",
	})

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res, tc.DeepEquals, params.UpgradeModelResult{
		ChosenVersion: version,
	})
}

// TestUpgradeModelWithVersionAndStream tests the dry run upgrade with
// an explicit version and stream. This is a happy case.
func (u *modelUpgradeSuite) TestUpgradeModelWithVersionAndStreamDryRun(c *tc.C) {
	ctrl := u.setup(c)
	defer ctrl.Finish()

	currentTargetVersion, err := semversion.Parse("4.0.0")
	c.Assert(err, tc.ErrorIsNil)

	desiredTargetVersion, err := semversion.Parse("4.0.1")
	c.Assert(err, tc.ErrorIsNil)

	u.authorizer.EXPECT().HasPermission(
		gomock.Any(),
		permission.SuperuserAccess,
		u.controllerTag,
	).Return(nil)
	u.check.EXPECT().ChangeAllowed(gomock.Any()).Return(nil)

	u.modelAgentService.EXPECT().RunPreUpgradeChecksToVersionWithStream(
		gomock.Any(),
		desiredTargetVersion,
		modelagent.AgentStreamReleased,
	).Return(currentTargetVersion, nil)

	api := NewModelUpgraderAPI(
		u.controllerTag,
		u.modelTag,
		u.authorizer, u.check, u.modelAgentService)

	res, err := api.UpgradeModel(c.Context(), params.UpgradeModelParams{
		ModelTag:      u.modelTag.String(),
		TargetVersion: desiredTargetVersion,
		AgentStream:   "released",
		DryRun:        true,
	})

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res, tc.DeepEquals, params.UpgradeModelResult{
		ChosenVersion: desiredTargetVersion,
	})
}

// TestUpgradeModelWithVersion tests the upgrade passing
// an explicit version. This is a happy case.
func (u *modelUpgradeSuite) TestUpgradeModelWithVersion(c *tc.C) {
	ctrl := u.setup(c)
	defer ctrl.Finish()

	version, err := semversion.Parse("4.0.1")
	c.Assert(err, tc.ErrorIsNil)

	u.authorizer.EXPECT().HasPermission(
		gomock.Any(),
		permission.SuperuserAccess,
		u.controllerTag,
	).Return(nil)
	u.check.EXPECT().ChangeAllowed(gomock.Any()).Return(nil)
	u.modelAgentService.EXPECT().UpgradeModelTargetAgentVersionTo(
		gomock.Any(),
		version,
	).Return(nil)

	api := NewModelUpgraderAPI(
		u.controllerTag,
		u.modelTag,
		u.authorizer, u.check, u.modelAgentService)

	res, err := api.UpgradeModel(c.Context(), params.UpgradeModelParams{
		ModelTag:      u.modelTag.String(),
		TargetVersion: version,
	})

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res, tc.DeepEquals, params.UpgradeModelResult{
		ChosenVersion: version,
	})
}

// TestUpgradeModelWithVersion tests the dry run upgrade passing
// an explicit version. This is a happy case.
func (u *modelUpgradeSuite) TestUpgradeModelWithVersionDryRun(c *tc.C) {
	ctrl := u.setup(c)
	defer ctrl.Finish()

	currentTargetVersion, err := semversion.Parse("4.0.0")
	c.Assert(err, tc.ErrorIsNil)

	desiredTargetVersion, err := semversion.Parse("4.0.1")
	c.Assert(err, tc.ErrorIsNil)

	u.authorizer.EXPECT().HasPermission(
		gomock.Any(),
		permission.SuperuserAccess,
		u.controllerTag,
	).Return(nil)
	u.check.EXPECT().ChangeAllowed(gomock.Any()).Return(nil)
	u.modelAgentService.EXPECT().RunPreUpgradeChecksToVersion(
		gomock.Any(),
		desiredTargetVersion,
	).Return(currentTargetVersion, nil)

	api := NewModelUpgraderAPI(
		u.controllerTag,
		u.modelTag,
		u.authorizer, u.check, u.modelAgentService)

	res, err := api.UpgradeModel(c.Context(), params.UpgradeModelParams{
		ModelTag:      u.modelTag.String(),
		TargetVersion: desiredTargetVersion,
		DryRun:        true,
	})

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res, tc.DeepEquals, params.UpgradeModelResult{
		ChosenVersion: desiredTargetVersion,
	})
}

// TestUpgradeModelWithStream tests the upgrade passing
// an explicit stream. This is a happy case.
func (u *modelUpgradeSuite) TestUpgradeModelWithStream(c *tc.C) {
	ctrl := u.setup(c)
	defer ctrl.Finish()

	version, err := semversion.Parse("4.0.1")
	c.Assert(err, tc.ErrorIsNil)

	u.authorizer.EXPECT().HasPermission(
		gomock.Any(),
		permission.SuperuserAccess,
		u.controllerTag,
	).Return(nil)
	u.check.EXPECT().ChangeAllowed(gomock.Any()).Return(nil)

	u.modelAgentService.EXPECT().UpgradeModelTargetAgentVersionStream(
		gomock.Any(),
		modelagent.AgentStreamReleased,
	).Return(version, nil)

	api := NewModelUpgraderAPI(
		u.controllerTag,
		u.modelTag,
		u.authorizer, u.check, u.modelAgentService)

	res, err := api.UpgradeModel(c.Context(), params.UpgradeModelParams{
		ModelTag:    u.modelTag.String(),
		AgentStream: "released",
	})

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res, tc.DeepEquals, params.UpgradeModelResult{
		ChosenVersion: version,
	})
}

// TestUpgradeModelWithStream tests the dry run upgrade passing
// an explicit stream. This is a happy case.
func (u *modelUpgradeSuite) TestUpgradeModelWithStreamDryRun(c *tc.C) {
	ctrl := u.setup(c)
	defer ctrl.Finish()

	version, err := semversion.Parse("4.0.1")
	c.Assert(err, tc.ErrorIsNil)

	u.authorizer.EXPECT().HasPermission(
		gomock.Any(),
		permission.SuperuserAccess,
		u.controllerTag,
	).Return(nil)
	u.check.EXPECT().ChangeAllowed(gomock.Any()).Return(nil)

	u.modelAgentService.EXPECT().RunPreUpgradeChecksWithStream(
		gomock.Any(), modelagent.AgentStreamReleased,
	).Return(version, nil)

	api := NewModelUpgraderAPI(
		u.controllerTag,
		u.modelTag,
		u.authorizer, u.check, u.modelAgentService)

	res, err := api.UpgradeModel(c.Context(), params.UpgradeModelParams{
		ModelTag:    u.modelTag.String(),
		AgentStream: "released",
		DryRun:      true,
	})

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res, tc.DeepEquals, params.UpgradeModelResult{
		ChosenVersion: version,
	})
}

// TestUpgradeModelWithoutVersionAndStream tests the upgrade without passing
// an explicit version and stream. This is a happy case.
func (u *modelUpgradeSuite) TestUpgradeModelWithoutVersionAndStream(c *tc.C) {
	ctrl := u.setup(c)
	defer ctrl.Finish()

	version, err := semversion.Parse("4.0.1")
	c.Assert(err, tc.ErrorIsNil)

	u.authorizer.EXPECT().HasPermission(
		gomock.Any(),
		permission.SuperuserAccess,
		u.controllerTag,
	).Return(nil)
	u.check.EXPECT().ChangeAllowed(gomock.Any()).Return(nil)
	u.modelAgentService.EXPECT().
		UpgradeModelTargetAgentVersion(gomock.Any()).Return(version, nil)

	api := NewModelUpgraderAPI(
		u.controllerTag,
		u.modelTag,
		u.authorizer, u.check, u.modelAgentService)

	res, err := api.UpgradeModel(c.Context(), params.UpgradeModelParams{
		ModelTag: u.modelTag.String(),
	})

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res, tc.DeepEquals, params.UpgradeModelResult{
		ChosenVersion: version,
	})
}

// TestUpgradeModelWithoutVersionAndStream tests the dry run upgrade without passing
// an explicit version and stream. This is a happy case.
func (u *modelUpgradeSuite) TestUpgradeModelWithoutVersionAndStreamDryRun(c *tc.C) {
	ctrl := u.setup(c)
	defer ctrl.Finish()

	version, err := semversion.Parse("4.0.1")
	c.Assert(err, tc.ErrorIsNil)

	u.authorizer.EXPECT().HasPermission(
		gomock.Any(),
		permission.SuperuserAccess,
		u.controllerTag,
	).Return(nil)
	u.check.EXPECT().ChangeAllowed(gomock.Any()).Return(nil)
	u.modelAgentService.EXPECT().
		RunPreUpgradeChecks(gomock.Any()).Return(version, nil)

	api := NewModelUpgraderAPI(
		u.controllerTag,
		u.modelTag,
		u.authorizer, u.check, u.modelAgentService)

	res, err := api.UpgradeModel(c.Context(), params.UpgradeModelParams{
		ModelTag: u.modelTag.String(),
		DryRun:   true,
	})

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res, tc.DeepEquals, params.UpgradeModelResult{
		ChosenVersion: version,
	})
}

// TestUpgradeModelMapErrMissingAgentBinariesToNotFound tests that the
// [modelagenterrors.MissingAgentBinaries] is mapped to a
// not found error.
// This is a sad case.
func (u *modelUpgradeSuite) TestUpgradeModelMapErrMissingAgentBinariesToNotFound(c *tc.C) {
	ctrl := u.setup(c)
	defer ctrl.Finish()

	u.authorizer.EXPECT().HasPermission(
		gomock.Any(),
		permission.SuperuserAccess,
		u.controllerTag,
	).Return(nil)
	u.check.EXPECT().ChangeAllowed(gomock.Any()).Return(nil)
	u.modelAgentService.EXPECT().UpgradeModelTargetAgentVersion(gomock.Any()).Return(
		semversion.Zero,
		errors.New("bad").
			Add(modelagenterrors.MissingAgentBinaries),
	)

	api := NewModelUpgraderAPI(
		u.controllerTag,
		u.modelTag,
		u.authorizer, u.check, u.modelAgentService)

	res, err := api.UpgradeModel(c.Context(), params.UpgradeModelParams{
		ModelTag: u.modelTag.String(),
	})

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res, tc.DeepEquals, params.UpgradeModelResult{
		Error: &params.Error{
			Message: "model agent binaries are not available for version \"0.0.0\"",
			Code:    "not found",
		},
	})
}

// TestUpgradeModelMapErrControllerUpgradeBlockerToNotSupported tests that the
// [controllerupgradererrors.ControllerUpgradeBlocker] is mapped to a
// not supported error.
// This is a sad case.
func (u *modelUpgradeSuite) TestUpgradeModelMapErrModelUpgradeBlockerToNotSupported(c *tc.C) {
	ctrl := u.setup(c)
	defer ctrl.Finish()

	u.authorizer.EXPECT().HasPermission(
		gomock.Any(),
		permission.SuperuserAccess,
		u.controllerTag,
	).Return(nil)
	u.check.EXPECT().ChangeAllowed(gomock.Any()).Return(nil)
	u.modelAgentService.EXPECT().UpgradeModelTargetAgentVersion(gomock.Any()).Return(
		semversion.Zero,
		modelagenterrors.ModelUpgradeBlocker{
			Reason: "model has 1 machines using unsupported bases",
		},
	)

	api := NewModelUpgraderAPI(
		u.controllerTag,
		u.modelTag,
		u.authorizer, u.check, u.modelAgentService)

	res, err := api.UpgradeModel(c.Context(), params.UpgradeModelParams{
		ModelTag: u.modelTag.String(),
	})

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res, tc.DeepEquals, params.UpgradeModelResult{
		Error: &params.Error{
			Message: "model upgrading is blocked for reason: model " +
				"has 1 machines using unsupported bases",
			Code: "not supported",
		},
	})
}

// TestUpgradeModelMapErrDowngradeNotSupportedToNotSupported tests that the
// [modelagenterrors.DowngradeNotSupported] is mapped to a
// not supported error.
// This is a sad case.
func (u *modelUpgradeSuite) TestUpgradeModelMapErrDowngradeNotSupportedToNotSupported(c *tc.C) {
	ctrl := u.setup(c)
	defer ctrl.Finish()

	u.authorizer.EXPECT().HasPermission(
		gomock.Any(),
		permission.SuperuserAccess,
		u.controllerTag,
	).Return(nil)
	u.check.EXPECT().ChangeAllowed(gomock.Any()).Return(nil)
	u.modelAgentService.EXPECT().UpgradeModelTargetAgentVersion(gomock.Any()).Return(
		semversion.Zero,
		modelagenterrors.DowngradeNotSupported,
	)

	api := NewModelUpgraderAPI(
		u.controllerTag,
		u.modelTag,
		u.authorizer, u.check, u.modelAgentService)

	res, err := api.UpgradeModel(c.Context(), params.UpgradeModelParams{
		ModelTag: u.modelTag.String(),
	})

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res, tc.DeepEquals, params.UpgradeModelResult{
		Error: &params.Error{
			Message: "cannot upgrade the model agent to version \"0.0.0\" because it is " +
				"lower than the current running version",
			Code: "not supported",
		},
	})
}

// TestUpgradeModelMapErrAgentVersionNotSupportedToNotValid tests that the
// [modelagenterrors.DowngradeNotSupported] is mapped to a
// not supported error.
// This is a sad case.
func (u *modelUpgradeSuite) TestUpgradeModelMapErrAgentVersionNotSupportedToNotValid(c *tc.C) {
	ctrl := u.setup(c)
	defer ctrl.Finish()

	u.authorizer.EXPECT().HasPermission(
		gomock.Any(),
		permission.SuperuserAccess,
		u.controllerTag,
	).Return(nil)
	u.check.EXPECT().ChangeAllowed(gomock.Any()).Return(nil)
	u.modelAgentService.EXPECT().UpgradeModelTargetAgentVersion(gomock.Any()).Return(
		semversion.Zero,
		modelagenterrors.AgentVersionNotSupported,
	)

	api := NewModelUpgraderAPI(
		u.controllerTag,
		u.modelTag,
		u.authorizer, u.check, u.modelAgentService)

	_, err := api.UpgradeModel(c.Context(), params.UpgradeModelParams{
		ModelTag: u.modelTag.String(),
	})

	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
}

// TestUpgradeModelMapErrInvalidStreamToNotValid tests that the
// [coreerrors.NotValid] is mapped to a not supported error.
// Based on the service contract, a [coreerrors.NotValid] is
// returned when the agent stream is not valid.
// This is a sad case.
func (u *modelUpgradeSuite) TestUpgradeModelMapErrInvalidStreamToNotValid(c *tc.C) {
	ctrl := u.setup(c)
	defer ctrl.Finish()

	u.authorizer.EXPECT().HasPermission(
		gomock.Any(),
		permission.SuperuserAccess,
		u.controllerTag,
	).Return(nil)
	u.check.EXPECT().ChangeAllowed(gomock.Any()).Return(nil)
	u.modelAgentService.EXPECT().UpgradeModelTargetAgentVersion(gomock.Any()).Return(
		semversion.Zero,
		modelagenterrors.AgentVersionNotSupported,
	)

	api := NewModelUpgraderAPI(
		u.controllerTag,
		u.modelTag,
		u.authorizer, u.check, u.modelAgentService)

	_, err := api.UpgradeModel(c.Context(), params.UpgradeModelParams{
		ModelTag: u.modelTag.String(),
	})

	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
}

// TestUpgradeModelMapOtherErrorsToServerError tests that the
// errors not defined in the switch case is mapped to a server error.
// This is a sad case.
func (u *modelUpgradeSuite) TestUpgradeModelMapOtherErrorsToServerError(c *tc.C) {
	ctrl := u.setup(c)
	defer ctrl.Finish()

	u.authorizer.EXPECT().HasPermission(
		gomock.Any(),
		permission.SuperuserAccess,
		u.controllerTag,
	).Return(nil)
	u.check.EXPECT().ChangeAllowed(gomock.Any()).Return(nil)
	u.modelAgentService.EXPECT().UpgradeModelTargetAgentVersion(
		gomock.Any(),
	).Return(
		semversion.Zero,
		errors.New("crazy error occurred"),
	)

	api := NewModelUpgraderAPI(
		u.controllerTag,
		u.modelTag,
		u.authorizer, u.check, u.modelAgentService)

	res, err := api.UpgradeModel(c.Context(), params.UpgradeModelParams{
		ModelTag: u.modelTag.String(),
	})

	c.Assert(err, tc.ErrorMatches,
		"crazy error occurred")
	c.Assert(res, tc.DeepEquals, params.UpgradeModelResult{})
}

// TestUpgradeModelNoWriteAccess tests that we get an error when the user
// doesn't have write access.
// This is a sad case.
func (u *modelUpgradeSuite) TestUpgradeModelNoWriteAccess(c *tc.C) {
	ctrl := u.setup(c)
	defer ctrl.Finish()

	u.authorizer.EXPECT().HasPermission(
		gomock.Any(),
		permission.SuperuserAccess,
		u.controllerTag,
	).Return(
		errors.New("not authorized").
			Add(authentication.ErrorEntityMissingPermission),
	)
	u.authorizer.EXPECT().HasPermission(
		gomock.Any(),
		permission.WriteAccess,
		u.modelTag).
		Return(
			errors.New("not authorized").
				Add(authentication.ErrorEntityMissingPermission),
		)

	api := NewModelUpgraderAPI(
		u.controllerTag,
		u.modelTag,
		u.authorizer, u.check, u.modelAgentService)

	res, err := api.UpgradeModel(c.Context(), params.UpgradeModelParams{
		ModelTag: u.modelTag.String(),
	})

	c.Assert(err, tc.ErrorMatches, "unauthorized to upgrade model")
	c.Assert(res, tc.DeepEquals, params.UpgradeModelResult{})
}

// TestUpgradeModelNoWriteAccess tests that we get an error when there is
// a change block in place.
// This is a sad case.
func (u *modelUpgradeSuite) TestUpgradeModelChangeNotAllowed(c *tc.C) {
	ctrl := u.setup(c)
	defer ctrl.Finish()

	u.authorizer.EXPECT().HasPermission(
		gomock.Any(),
		permission.SuperuserAccess,
		u.controllerTag,
	).Return(nil)
	u.check.EXPECT().ChangeAllowed(gomock.Any()).Return(errors.New("not allowed"))

	api := NewModelUpgraderAPI(
		u.controllerTag,
		u.modelTag,
		u.authorizer, u.check, u.modelAgentService)

	res, err := api.UpgradeModel(c.Context(), params.UpgradeModelParams{
		ModelTag: u.modelTag.String(),
	})

	c.Assert(err, tc.ErrorMatches, "not allowed")
	c.Assert(res, tc.DeepEquals, params.UpgradeModelResult{})
}

// TestUpgradeModelErrorBecauseOfDifferentModel tests that we get
// an error when the given model tag is different to the hosted model.
// This is a sad case.
func (u *modelUpgradeSuite) TestUpgradeModelErrorBecauseOfDifferentModel(c *tc.C) {
	ctrl := u.setup(c)
	defer ctrl.Finish()

	api := NewModelUpgraderAPI(
		u.controllerTag,
		u.modelTag,
		u.authorizer, u.check, u.modelAgentService)

	res, err := api.UpgradeModel(c.Context(), params.UpgradeModelParams{
		ModelTag: names.NewModelTag(uuid.MustNewUUID().String()).String(),
	})

	c.Assert(err, tc.ErrorMatches, "unauthorized to upgrade model")
	c.Assert(res, tc.DeepEquals, params.UpgradeModelResult{})
}

// TestUpgradeModelErrorModelTag tests that we get an error when
// a poorly formatted model tag is given.
// This is a sad case.
func (u *modelUpgradeSuite) TestUpgradeModelErrorModelTag(c *tc.C) {
	ctrl := u.setup(c)
	defer ctrl.Finish()

	api := NewModelUpgraderAPI(
		u.controllerTag,
		u.modelTag,
		u.authorizer, u.check, u.modelAgentService)

	res, err := api.UpgradeModel(c.Context(), params.UpgradeModelParams{
		ModelTag: names.NewModelTag("broken-uuid").String(),
	})

	c.Assert(err, tc.ErrorMatches,
		`"model-broken-uuid" is not a valid model tag`)
	c.Assert(res, tc.DeepEquals, params.UpgradeModelResult{})
}

// TestUpgradeModelErrorCanUpgrade tests that we get an error when
// [ControllerUpgraderAPI.canUpgrade] func returns a non-permission error.
// This is a sad case.
func (u *modelUpgradeSuite) TestUpgradeModelErrorCanUpgrade(c *tc.C) {
	ctrl := u.setup(c)
	defer ctrl.Finish()

	u.authorizer.EXPECT().HasPermission(
		gomock.Any(),
		permission.SuperuserAccess,
		u.controllerTag,
	).Return(errors.New("unknown failure"))

	api := NewModelUpgraderAPI(
		u.controllerTag,
		u.modelTag,
		u.authorizer, u.check, u.modelAgentService)

	res, err := api.UpgradeModel(c.Context(), params.UpgradeModelParams{
		ModelTag: u.modelTag.String(),
	})

	c.Assert(err, tc.ErrorMatches, "unknown failure")
	c.Assert(res, tc.DeepEquals, params.UpgradeModelResult{})
}

// TestUpgradeModelErrorCanUpgrade tests that we correctly map the error when
// the given stream fails to parse.
// This is a sad case.
func (u *modelUpgradeSuite) TestUpgradeModelErrUnknownStreamMapToNotValid(c *tc.C) {
	ctrl := u.setup(c)
	defer ctrl.Finish()

	u.authorizer.EXPECT().HasPermission(
		gomock.Any(),
		permission.SuperuserAccess,
		u.controllerTag,
	).Return(nil)
	u.check.EXPECT().ChangeAllowed(gomock.Any()).Return(nil)

	api := NewModelUpgraderAPI(
		u.controllerTag,
		u.modelTag,
		u.authorizer, u.check, u.modelAgentService)

	res, err := api.UpgradeModel(c.Context(), params.UpgradeModelParams{
		ModelTag:    u.modelTag.String(),
		AgentStream: "unknownstream",
	})

	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
	c.Assert(res, tc.DeepEquals, params.UpgradeModelResult{})
}
