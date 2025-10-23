// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelupgrader

import (
	"testing"

	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/apiserver/authentication"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade/mocks"
	modelupgradermocks "github.com/juju/juju/apiserver/facades/client/modelupgrader/mocks"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/semversion"
	domainagentbinary "github.com/juju/juju/domain/agentbinary"
	controllerupgradererrors "github.com/juju/juju/domain/controllerupgrader/errors"
	modelagenterrors "github.com/juju/juju/domain/modelagent/errors"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/uuid"
	"github.com/juju/juju/rpc/params"
)

type controllerUpgraderAPISuite struct {
	authorizer      *mocks.MockAuthorizer
	check           *modelupgradermocks.MockBlockCheckerInterface
	upgraderService *modelupgradermocks.MockControllerUpgraderService
	controllerTag   names.Tag
	modelTag        names.Tag
}

// TestControllerUpgraderAPISuite runs the test methods in controllerUpgraderAPISuite.
func TestControllerUpgraderAPISuite(t *testing.T) {
	tc.Run(t, &controllerUpgraderAPISuite{})
}

// setup instantiates the mocked dependencies.
func (u *controllerUpgraderAPISuite) setup(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	u.authorizer = mocks.NewMockAuthorizer(ctrl)
	u.check = modelupgradermocks.NewMockBlockCheckerInterface(ctrl)
	u.upgraderService = modelupgradermocks.NewMockControllerUpgraderService(ctrl)
	u.controllerTag = names.NewControllerTag(tc.Must(c, uuid.NewUUID).String())
	u.modelTag = names.NewModelTag(tc.Must(c, uuid.NewUUID).String())

	c.Cleanup(func() {
		u.authorizer = nil
		u.check = nil
		u.upgraderService = nil
		u.controllerTag = nil
		u.modelTag = nil
	})
	return ctrl
}

// TestUpgradeModelWithVersionAndStream tests the upgrade with
// an explicit version and stream. This is a happy case.
func (u *controllerUpgraderAPISuite) TestUpgradeModelWithVersionAndStream(c *tc.C) {
	ctrl := u.setup(c)
	defer ctrl.Finish()

	version, err := semversion.Parse("4.0.1")
	c.Assert(err, tc.ErrorIsNil)

	u.authorizer.EXPECT().AuthClient().Return(true)
	u.authorizer.EXPECT().HasPermission(
		gomock.Any(),
		permission.SuperuserAccess,
		u.controllerTag,
	).Return(nil)
	u.check.EXPECT().ChangeAllowed(gomock.Any()).Return(nil)

	u.upgraderService.EXPECT().UpgradeControllerToVersionWithStream(
		gomock.Any(),
		version,
		domainagentbinary.AgentStreamReleased,
	).Return(nil)

	api, err := NewControllerUpgraderAPI(
		u.controllerTag,
		u.modelTag,
		u.authorizer, u.check, u.upgraderService)
	c.Assert(err, tc.ErrorIsNil)

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
func (u *controllerUpgraderAPISuite) TestUpgradeModelWithVersionAndStreamDryRun(c *tc.C) {
	ctrl := u.setup(c)
	defer ctrl.Finish()

	version, err := semversion.Parse("4.0.1")
	c.Assert(err, tc.ErrorIsNil)

	u.authorizer.EXPECT().AuthClient().Return(true)
	u.authorizer.EXPECT().HasPermission(
		gomock.Any(),
		permission.SuperuserAccess,
		u.controllerTag,
	).Return(nil)
	u.check.EXPECT().ChangeAllowed(gomock.Any()).Return(nil)

	u.upgraderService.EXPECT().RunPreUpgradeChecksToVersionWithStream(
		gomock.Any(),
		version,
		domainagentbinary.AgentStreamReleased,
	).Return(nil)

	api, err := NewControllerUpgraderAPI(
		u.controllerTag,
		u.modelTag,
		u.authorizer, u.check, u.upgraderService)
	c.Assert(err, tc.ErrorIsNil)

	res, err := api.UpgradeModel(c.Context(), params.UpgradeModelParams{
		ModelTag:      u.modelTag.String(),
		TargetVersion: version,
		AgentStream:   "released",
		DryRun:        true,
	})

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res, tc.DeepEquals, params.UpgradeModelResult{
		ChosenVersion: version,
	})
}

// TestUpgradeModelWithVersion tests the upgrade passing
// an explicit version. This is a happy case.
func (u *controllerUpgraderAPISuite) TestUpgradeModelWithVersion(c *tc.C) {
	ctrl := u.setup(c)
	defer ctrl.Finish()

	version, err := semversion.Parse("4.0.1")
	c.Assert(err, tc.ErrorIsNil)

	u.authorizer.EXPECT().AuthClient().Return(true)
	u.authorizer.EXPECT().HasPermission(
		gomock.Any(),
		permission.SuperuserAccess,
		u.controllerTag,
	).Return(nil)
	u.check.EXPECT().ChangeAllowed(gomock.Any()).Return(nil)
	u.upgraderService.EXPECT().UpgradeControllerToVersion(
		gomock.Any(),
		version,
	).Return(nil)

	api, err := NewControllerUpgraderAPI(
		u.controllerTag,
		u.modelTag,
		u.authorizer, u.check, u.upgraderService)
	c.Assert(err, tc.ErrorIsNil)

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
func (u *controllerUpgraderAPISuite) TestUpgradeModelWithVersionDryRun(c *tc.C) {
	ctrl := u.setup(c)
	defer ctrl.Finish()

	version, err := semversion.Parse("4.0.1")
	c.Assert(err, tc.ErrorIsNil)

	u.authorizer.EXPECT().AuthClient().Return(true)
	u.authorizer.EXPECT().HasPermission(
		gomock.Any(),
		permission.SuperuserAccess,
		u.controllerTag,
	).Return(nil)
	u.check.EXPECT().ChangeAllowed(gomock.Any()).Return(nil)
	u.upgraderService.EXPECT().RunPreUpgradeChecksToVersion(
		gomock.Any(),
		version,
	).Return(nil)

	api, err := NewControllerUpgraderAPI(
		u.controllerTag,
		u.modelTag,
		u.authorizer, u.check, u.upgraderService)
	c.Assert(err, tc.ErrorIsNil)

	res, err := api.UpgradeModel(c.Context(), params.UpgradeModelParams{
		ModelTag:      u.modelTag.String(),
		TargetVersion: version,
		DryRun:        true,
	})

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res, tc.DeepEquals, params.UpgradeModelResult{
		ChosenVersion: version,
	})
}

// TestUpgradeModelWithStream tests the upgrade passing
// an explicit stream. This is a happy case.
func (u *controllerUpgraderAPISuite) TestUpgradeModelWithStream(c *tc.C) {
	ctrl := u.setup(c)
	defer ctrl.Finish()

	version, err := semversion.Parse("4.0.1")
	c.Assert(err, tc.ErrorIsNil)

	u.authorizer.EXPECT().AuthClient().Return(true)
	u.authorizer.EXPECT().HasPermission(
		gomock.Any(),
		permission.SuperuserAccess,
		u.controllerTag,
	).Return(nil)
	u.check.EXPECT().ChangeAllowed(gomock.Any()).Return(nil)

	u.upgraderService.EXPECT().UpgradeControllerWithStream(
		gomock.Any(),
		domainagentbinary.AgentStreamReleased,
	).Return(version, nil)

	api, err := NewControllerUpgraderAPI(
		u.controllerTag,
		u.modelTag,
		u.authorizer, u.check, u.upgraderService)
	c.Assert(err, tc.ErrorIsNil)

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
func (u *controllerUpgraderAPISuite) TestUpgradeModelWithStreamDryRun(c *tc.C) {
	ctrl := u.setup(c)
	defer ctrl.Finish()

	version, err := semversion.Parse("4.0.1")
	c.Assert(err, tc.ErrorIsNil)

	u.authorizer.EXPECT().AuthClient().Return(true)
	u.authorizer.EXPECT().HasPermission(
		gomock.Any(),
		permission.SuperuserAccess,
		u.controllerTag,
	).Return(nil)
	u.check.EXPECT().ChangeAllowed(gomock.Any()).Return(nil)

	u.upgraderService.EXPECT().RunPreUpgradeChecksWithStream(
		gomock.Any(), domainagentbinary.AgentStreamReleased,
	).Return(version, nil)

	api, err := NewControllerUpgraderAPI(
		u.controllerTag,
		u.modelTag,
		u.authorizer, u.check, u.upgraderService)
	c.Assert(err, tc.ErrorIsNil)

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
func (u *controllerUpgraderAPISuite) TestUpgradeModelWithoutVersionAndStream(c *tc.C) {
	ctrl := u.setup(c)
	defer ctrl.Finish()

	version, err := semversion.Parse("4.0.1")
	c.Assert(err, tc.ErrorIsNil)

	u.authorizer.EXPECT().AuthClient().Return(true)
	u.authorizer.EXPECT().HasPermission(
		gomock.Any(),
		permission.SuperuserAccess,
		u.controllerTag,
	).Return(nil)
	u.check.EXPECT().ChangeAllowed(gomock.Any()).Return(nil)
	u.upgraderService.EXPECT().
		UpgradeController(gomock.Any()).Return(version, nil)

	api, err := NewControllerUpgraderAPI(
		u.controllerTag,
		u.modelTag,
		u.authorizer, u.check, u.upgraderService)
	c.Assert(err, tc.ErrorIsNil)

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
func (u *controllerUpgraderAPISuite) TestUpgradeModelWithoutVersionAndStreamDryRun(c *tc.C) {
	ctrl := u.setup(c)
	defer ctrl.Finish()

	version, err := semversion.Parse("4.0.1")
	c.Assert(err, tc.ErrorIsNil)

	u.authorizer.EXPECT().AuthClient().Return(true)
	u.authorizer.EXPECT().HasPermission(
		gomock.Any(),
		permission.SuperuserAccess,
		u.controllerTag,
	).Return(nil)
	u.check.EXPECT().ChangeAllowed(gomock.Any()).Return(nil)
	u.upgraderService.EXPECT().
		RunPreUpgradeChecks(gomock.Any()).Return(version, nil)

	api, err := NewControllerUpgraderAPI(
		u.controllerTag,
		u.modelTag,
		u.authorizer, u.check, u.upgraderService)
	c.Assert(err, tc.ErrorIsNil)

	res, err := api.UpgradeModel(c.Context(), params.UpgradeModelParams{
		ModelTag: u.modelTag.String(),
		DryRun:   true,
	})

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res, tc.DeepEquals, params.UpgradeModelResult{
		ChosenVersion: version,
	})
}

// TestUpgradeModelMapErrMissingControllerBinariesToNotFound tests that the
// [controllerupgradererrors.MissingControllerBinaries] is mapped to a
// not found error.
// This is a sad case.
func (u *controllerUpgraderAPISuite) TestUpgradeModelMapErrMissingControllerBinariesToNotFound(c *tc.C) {
	ctrl := u.setup(c)
	defer ctrl.Finish()

	u.authorizer.EXPECT().AuthClient().Return(true)
	u.authorizer.EXPECT().HasPermission(
		gomock.Any(),
		permission.SuperuserAccess,
		u.controllerTag,
	).Return(nil)
	u.check.EXPECT().ChangeAllowed(gomock.Any()).Return(nil)
	u.upgraderService.EXPECT().UpgradeController(gomock.Any()).Return(
		semversion.Zero,
		errors.New("bad").
			Add(controllerupgradererrors.MissingControllerBinaries),
	)

	api, err := NewControllerUpgraderAPI(
		u.controllerTag,
		u.modelTag,
		u.authorizer, u.check, u.upgraderService)
	c.Assert(err, tc.ErrorIsNil)

	res, err := api.UpgradeModel(c.Context(), params.UpgradeModelParams{
		ModelTag: u.modelTag.String(),
	})

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res, tc.DeepEquals, params.UpgradeModelResult{
		Error: &params.Error{
			Message: "controller agent binaries are not available for version \"0.0.0\"",
			Code:    "not found",
		},
	})
}

// TestUpgradeModelMapErrControllerUpgradeBlockerToNotSupported tests that the
// [controllerupgradererrors.ControllerUpgradeBlocker] is mapped to a
// not supported error.
// This is a sad case.
func (u *controllerUpgraderAPISuite) TestUpgradeModelMapErrControllerUpgradeBlockerToNotSupported(c *tc.C) {
	ctrl := u.setup(c)
	defer ctrl.Finish()

	u.authorizer.EXPECT().AuthClient().Return(true)
	u.authorizer.EXPECT().HasPermission(
		gomock.Any(),
		permission.SuperuserAccess,
		u.controllerTag,
	).Return(nil)
	u.check.EXPECT().ChangeAllowed(gomock.Any()).Return(nil)
	u.upgraderService.EXPECT().UpgradeController(gomock.Any()).Return(
		semversion.Zero,
		controllerupgradererrors.ControllerUpgradeBlocker{
			Reason: "controller nodes 1 are not running controller version 4.0.1",
		},
	)

	api, err := NewControllerUpgraderAPI(
		u.controllerTag,
		u.modelTag,
		u.authorizer, u.check, u.upgraderService)
	c.Assert(err, tc.ErrorIsNil)

	res, err := api.UpgradeModel(c.Context(), params.UpgradeModelParams{
		ModelTag: u.modelTag.String(),
	})

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res, tc.DeepEquals, params.UpgradeModelResult{
		Error: &params.Error{
			Message: "controller upgrading is blocked for reason: controller " +
				"nodes 1 are not running controller version 4.0.1",
			Code: "not supported",
		},
	})
}

// TestUpgradeModelMapErrVersionNotSupportedToNotValid tests that the
// [controllerupgradererrors.VersionNotSupported] is mapped to a
// [coreerrors.NotValid]. This is a sad case.
func (u *controllerUpgraderAPISuite) TestUpgradeModelMapErrVersionNotSupportedToNotValid(c *tc.C) {
	ctrl := u.setup(c)
	defer ctrl.Finish()

	u.authorizer.EXPECT().AuthClient().Return(true)
	u.authorizer.EXPECT().HasPermission(
		gomock.Any(),
		permission.SuperuserAccess,
		u.controllerTag,
	).Return(nil)
	u.check.EXPECT().ChangeAllowed(gomock.Any()).Return(nil)
	u.upgraderService.EXPECT().UpgradeController(gomock.Any()).Return(
		semversion.Zero,
		errors.New("bad").Add(controllerupgradererrors.VersionNotSupported))

	api, err := NewControllerUpgraderAPI(
		u.controllerTag,
		u.modelTag,
		u.authorizer, u.check, u.upgraderService)
	c.Assert(err, tc.ErrorIsNil)

	res, err := api.UpgradeModel(c.Context(), params.UpgradeModelParams{
		ModelTag: u.modelTag.String(),
	})

	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
	c.Assert(res, tc.DeepEquals, params.UpgradeModelResult{})
}

// TestUpgradeModelMapErrAgentStreamNotValidToNotValid tests that the
// [modelagenterrors.AgentStreamNotValid] is mapped to a [coreerrors.NotValid].
// This is a sad case.
func (u *controllerUpgraderAPISuite) TestUpgradeModelMapErrAgentStreamNotValidToNotValid(c *tc.C) {
	ctrl := u.setup(c)
	defer ctrl.Finish()

	u.authorizer.EXPECT().AuthClient().Return(true)
	u.authorizer.EXPECT().HasPermission(
		gomock.Any(),
		permission.SuperuserAccess,
		u.controllerTag,
	).Return(nil)
	u.check.EXPECT().ChangeAllowed(gomock.Any()).Return(nil)
	u.upgraderService.EXPECT().UpgradeController(gomock.Any()).Return(
		semversion.Zero,
		errors.New("bad").Add(modelagenterrors.AgentStreamNotValid),
	)

	api, err := NewControllerUpgraderAPI(
		u.controllerTag,
		u.modelTag,
		u.authorizer, u.check, u.upgraderService)
	c.Assert(err, tc.ErrorIsNil)

	res, err := api.UpgradeModel(c.Context(), params.UpgradeModelParams{
		ModelTag: u.modelTag.String(),
	})

	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
	c.Assert(res, tc.DeepEquals, params.UpgradeModelResult{})
}

// TestUpgradeModelMapOtherErrorsToServerError tests that the
// [controllerupgradererrors.DowngradeNotSupported] is mapped to a not
// supported error. This is a sad case.
func (u *controllerUpgraderAPISuite) TestUpgradeModelMapErrDowngradeNotSupportedToNotSupported(c *tc.C) {
	ctrl := u.setup(c)
	defer ctrl.Finish()

	u.authorizer.EXPECT().AuthClient().Return(true)
	u.authorizer.EXPECT().HasPermission(
		gomock.Any(),
		permission.SuperuserAccess,
		u.controllerTag,
	).Return(nil)
	u.check.EXPECT().ChangeAllowed(gomock.Any()).Return(nil)
	u.upgraderService.EXPECT().UpgradeController(gomock.Any()).Return(
		semversion.Zero,
		errors.New("controller version downgrades are not supported").
			Add(controllerupgradererrors.DowngradeNotSupported),
	)

	api, err := NewControllerUpgraderAPI(
		u.controllerTag,
		u.modelTag,
		u.authorizer, u.check, u.upgraderService)
	c.Assert(err, tc.ErrorIsNil)

	res, err := api.UpgradeModel(c.Context(), params.UpgradeModelParams{
		ModelTag: u.modelTag.String(),
	})

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res, tc.DeepEquals, params.UpgradeModelResult{
		Error: &params.Error{
			Message: "cannot upgrade the controller to version \"0.0.0\" because it is " +
				"lower than the current running version",
			Code: "not supported",
		},
	})
}

// TestUpgradeModelMapOtherErrorsToServerError tests that the
// errors not defined in the switch case is mapped to a server error.
// This is a sad case.
func (u *controllerUpgraderAPISuite) TestUpgradeModelMapOtherErrorsToServerError(c *tc.C) {
	ctrl := u.setup(c)
	defer ctrl.Finish()

	u.authorizer.EXPECT().AuthClient().Return(true)
	u.authorizer.EXPECT().HasPermission(
		gomock.Any(),
		permission.SuperuserAccess,
		u.controllerTag,
	).Return(nil)
	u.check.EXPECT().ChangeAllowed(gomock.Any()).Return(nil)
	u.upgraderService.EXPECT().UpgradeController(gomock.Any()).Return(
		semversion.Zero,
		errors.New("crazy error occurred"),
	)

	api, err := NewControllerUpgraderAPI(
		u.controllerTag,
		u.modelTag,
		u.authorizer, u.check, u.upgraderService)
	c.Assert(err, tc.ErrorIsNil)

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
func (u *controllerUpgraderAPISuite) TestUpgradeModelNoWriteAccess(c *tc.C) {
	ctrl := u.setup(c)
	defer ctrl.Finish()

	u.authorizer.EXPECT().AuthClient().Return(true)
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

	api, err := NewControllerUpgraderAPI(
		u.controllerTag,
		u.modelTag,
		u.authorizer, u.check, u.upgraderService)
	c.Assert(err, tc.ErrorIsNil)

	res, err := api.UpgradeModel(c.Context(), params.UpgradeModelParams{
		ModelTag: u.modelTag.String(),
	})

	c.Assert(err, tc.ErrorMatches, "unauthorized to upgrade model")
	c.Assert(res, tc.DeepEquals, params.UpgradeModelResult{})
}

// TestUpgradeModelNoWriteAccess tests that we get an error when there is
// a change block in place.
// This is a sad case.
func (u *controllerUpgraderAPISuite) TestUpgradeModelChangeNotAllowed(c *tc.C) {
	ctrl := u.setup(c)
	defer ctrl.Finish()

	u.authorizer.EXPECT().AuthClient().Return(true)
	u.authorizer.EXPECT().HasPermission(
		gomock.Any(),
		permission.SuperuserAccess,
		u.controllerTag,
	).Return(nil)
	u.check.EXPECT().ChangeAllowed(gomock.Any()).Return(errors.New("not allowed"))

	api, err := NewControllerUpgraderAPI(
		u.controllerTag,
		u.modelTag,
		u.authorizer, u.check, u.upgraderService)
	c.Assert(err, tc.ErrorIsNil)

	res, err := api.UpgradeModel(c.Context(), params.UpgradeModelParams{
		ModelTag: u.modelTag.String(),
	})

	c.Assert(err, tc.ErrorMatches, "not allowed")
	c.Assert(res, tc.DeepEquals, params.UpgradeModelResult{})
}

// TestUpgradeModelErrorBecauseOfDifferentModel tests that we get
// an error when the given model tag is different to the hosted model.
// This is a sad case.
func (u *controllerUpgraderAPISuite) TestUpgradeModelErrorBecauseOfDifferentModel(c *tc.C) {
	ctrl := u.setup(c)
	defer ctrl.Finish()

	u.authorizer.EXPECT().AuthClient().Return(true)

	api, err := NewControllerUpgraderAPI(
		u.controllerTag,
		u.modelTag,
		u.authorizer, u.check, u.upgraderService)
	c.Assert(err, tc.ErrorIsNil)

	res, err := api.UpgradeModel(c.Context(), params.UpgradeModelParams{
		ModelTag: names.NewModelTag(uuid.MustNewUUID().String()).String(),
	})

	c.Assert(err, tc.ErrorMatches, "unauthorized to upgrade model")
	c.Assert(res, tc.DeepEquals, params.UpgradeModelResult{})
}

// TestUpgradeModelErrorModelTag tests that we get an error when
// a poorly formatted model tag is given.
// This is a sad case.
func (u *controllerUpgraderAPISuite) TestUpgradeModelErrorModelTag(c *tc.C) {
	ctrl := u.setup(c)
	defer ctrl.Finish()

	u.authorizer.EXPECT().AuthClient().Return(true)

	api, err := NewControllerUpgraderAPI(
		u.controllerTag,
		u.modelTag,
		u.authorizer, u.check, u.upgraderService)
	c.Assert(err, tc.ErrorIsNil)

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
func (u *controllerUpgraderAPISuite) TestUpgradeModelErrorCanUpgrade(c *tc.C) {
	ctrl := u.setup(c)
	defer ctrl.Finish()

	u.authorizer.EXPECT().AuthClient().Return(true)
	u.authorizer.EXPECT().HasPermission(
		gomock.Any(),
		permission.SuperuserAccess,
		u.controllerTag,
	).Return(errors.New("unknown failure"))

	api, err := NewControllerUpgraderAPI(
		u.controllerTag,
		u.modelTag,
		u.authorizer, u.check, u.upgraderService)
	c.Assert(err, tc.ErrorIsNil)

	res, err := api.UpgradeModel(c.Context(), params.UpgradeModelParams{
		ModelTag: u.modelTag.String(),
	})

	c.Assert(err, tc.ErrorMatches, "unknown failure")
	c.Assert(res, tc.DeepEquals, params.UpgradeModelResult{})
}

// TestUpgradeModelErrorCanUpgrade tests that we correctly map the error when
// the given stream fails to parse.
// This is a sad case.
func (u *controllerUpgraderAPISuite) TestUpgradeModelErrUnknownStreamMapToNotValid(c *tc.C) {
	ctrl := u.setup(c)
	defer ctrl.Finish()

	u.authorizer.EXPECT().AuthClient().Return(true)
	u.authorizer.EXPECT().HasPermission(
		gomock.Any(),
		permission.SuperuserAccess,
		u.controllerTag,
	).Return(nil)
	u.check.EXPECT().ChangeAllowed(gomock.Any()).Return(nil)

	api, err := NewControllerUpgraderAPI(
		u.controllerTag,
		u.modelTag,
		u.authorizer, u.check, u.upgraderService)
	c.Assert(err, tc.ErrorIsNil)

	res, err := api.UpgradeModel(c.Context(), params.UpgradeModelParams{
		ModelTag:    u.modelTag.String(),
		AgentStream: "unknownstream",
	})

	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
	c.Assert(res, tc.DeepEquals, params.UpgradeModelResult{})
}

// TestNewControllerUpgraderAPIAuthError tests that an error is returned
// if the entity is not a client user while instantiating the
// controller upgrader API.
func (u *controllerUpgraderAPISuite) TestNewControllerUpgraderAPIAuthError(c *tc.C) {
	ctrl := u.setup(c)
	defer ctrl.Finish()

	u.authorizer.EXPECT().AuthClient().Return(false)

	_, err := NewControllerUpgraderAPI(
		u.controllerTag,
		u.modelTag,
		u.authorizer, u.check, u.upgraderService)
	c.Assert(err, tc.ErrorIs, apiservererrors.ErrPerm)
}

// AbortModelUpgrade tests that aborting a model upgrade is not supported.
func (u *controllerUpgraderAPISuite) AbortModelUpgrade(c *tc.C) {
	ctrl := u.setup(c)
	defer ctrl.Finish()

	u.authorizer.EXPECT().AuthClient().Return(true)

	api, err := NewControllerUpgraderAPI(
		u.controllerTag,
		u.modelTag,
		u.authorizer, u.check, u.upgraderService)
	c.Assert(err, tc.ErrorIsNil)

	err = api.AbortModelUpgrade(c.Context(), params.ModelParam{})

	c.Assert(err, tc.ErrorIs, coreerrors.NotSupported)
}
