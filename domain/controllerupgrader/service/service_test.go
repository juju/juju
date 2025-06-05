// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"testing"

	"github.com/juju/tc"
	gomock "go.uber.org/mock/gomock"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/semversion"
	controllerupgradererrors "github.com/juju/juju/domain/controllerupgrader/errors"
	"github.com/juju/juju/domain/modelagent"
	modelagenterrors "github.com/juju/juju/domain/modelagent/errors"
	"github.com/juju/juju/internal/errors"
)

type serviceSuite struct {
	agentBinaryFinder *MockAgentBinaryFinder
	ctrlSt            *MockControllerState
	modelSt           *MockModelState
}

// TestServiceSuite runs all of the tests located in the [serviceSuite].
func TestServiceSuite(t *testing.T) {
	tc.Run(t, &serviceSuite{})
}

// setupMocks initializes the mock objects for this suite.
func (s *serviceSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.agentBinaryFinder = NewMockAgentBinaryFinder(ctrl)
	s.ctrlSt = NewMockControllerState(ctrl)
	s.modelSt = NewMockModelState(ctrl)
	return ctrl
}

// TearDownTest cleans up the mock objects after each test.
func (s *serviceSuite) TearDownTest(c *tc.C) {
	s.agentBinaryFinder = nil
	s.ctrlSt = nil
	s.modelSt = nil
}

// TestUpgradeController tests the happy path for upgrading a controller to the
// latest available patch version.
func (s *serviceSuite) TestUpgradeController(c *tc.C) {
	defer s.setupMocks(c).Finish()

	highestVersion, err := semversion.Parse("4.0.7")
	c.Assert(err, tc.ErrorIsNil)
	currentControllerVersion, err := semversion.Parse("4.0.4")
	c.Assert(err, tc.ErrorIsNil)

	s.agentBinaryFinder.EXPECT().GetHighestPatchVersionAvailable(gomock.Any()).
		Return(highestVersion, nil)
	s.agentBinaryFinder.EXPECT().HasBinariesForVersion(
		gomock.Any(), highestVersion,
	).Return(true, nil)
	s.ctrlSt.EXPECT().GetControllerVersion(gomock.Any()).Return(
		currentControllerVersion, nil,
	)
	s.ctrlSt.EXPECT().GetControllerNodeVersions(gomock.Any()).Return(
		map[string]semversion.Number{
			"1": currentControllerVersion,
			"2": currentControllerVersion,
			"3": currentControllerVersion,
		}, nil,
	)
	s.modelSt.EXPECT().GetModelTargetAgentVersion(gomock.Any()).Return(
		currentControllerVersion, nil,
	)
	s.modelSt.EXPECT().SetModelTargetAgentVersion(
		gomock.Any(), currentControllerVersion, highestVersion,
	).Return(nil)
	s.ctrlSt.EXPECT().SetControllerVersion(gomock.Any(), highestVersion).Return(nil)

	svc := NewService(s.agentBinaryFinder, s.ctrlSt, s.modelSt)
	upgradedVer, err := svc.UpgradeController(c.Context())
	c.Check(err, tc.ErrorIsNil)
	c.Check(upgradedVer, tc.Equals, highestVersion)
}

// TestUpgradeControllerNodeBlocker tests the case where a controller upgrade
// is requested but one or more controller nodes exist that are not running the
// current controller version. In this case, the upgrade must fail with the
// caller getting back an error satisfying
// [controllerupgradererrors.ControllerUpgradeBlocker].
func (s *serviceSuite) TestUpgradeControllerNodeBlocker(c *tc.C) {
	defer s.setupMocks(c).Finish()

	highestVersion, err := semversion.Parse("4.0.7")
	c.Assert(err, tc.ErrorIsNil)
	currentControllerVersion, err := semversion.Parse("4.0.4")
	c.Assert(err, tc.ErrorIsNil)
	oldNodeVersion, err := semversion.Parse("4.0.2")
	c.Assert(err, tc.ErrorIsNil)

	s.agentBinaryFinder.EXPECT().GetHighestPatchVersionAvailable(gomock.Any()).
		Return(highestVersion, nil)
	s.ctrlSt.EXPECT().GetControllerVersion(gomock.Any()).Return(
		currentControllerVersion, nil,
	)
	s.ctrlSt.EXPECT().GetControllerNodeVersions(gomock.Any()).Return(
		map[string]semversion.Number{
			"1": oldNodeVersion,
			"2": currentControllerVersion,
			"3": oldNodeVersion,
		}, nil,
	)
	s.modelSt.EXPECT().GetModelTargetAgentVersion(gomock.Any()).Return(
		currentControllerVersion, nil,
	)

	svc := NewService(s.agentBinaryFinder, s.ctrlSt, s.modelSt)
	_, err = svc.UpgradeController(c.Context())
	blocker, is := errors.AsType[controllerupgradererrors.ControllerUpgradeBlocker](err)
	c.Check(is, tc.IsTrue)
	c.Check(blocker.Reason, tc.Matches, "controller nodes \\[(3 1|1 3)\\] are not running controller version \"4\\.0\\.4\"")
}

// TestUpgradeControllerWithInvalidStream tests the case where a controller
// upgrade is requested to the latest available version but the agent stream
// supplied is not valid. In this case, the upgrade must fail with an error and
// the caller gets back an error satisfying
// [modelagenterrors.AgentStreamNotValid].
func (s *serviceSuite) TestUpgradeControllerWithInvalidStream(c *tc.C) {
	defer s.setupMocks(c).Finish()

	svc := NewService(s.agentBinaryFinder, s.ctrlSt, s.modelSt)
	_, err := svc.UpgradeControllerWithStream(c.Context(), modelagent.AgentStream(-1))
	c.Check(err, tc.ErrorIs, modelagenterrors.AgentStreamNotValid)
}

// TestUpgradeControllerWithStreamNodeBlocker tests the case where a controller
// upgrade is requested but one or more controller nodes exist that are not
// running the current controller version. In this case, the upgrade must fail
// with the caller getting back an error satisfying
// [controllerupgradererrors.ControllerUpgradeBlocker].
func (s *serviceSuite) TestUpgradeControllerWithStreamNodeBlocker(c *tc.C) {
	defer s.setupMocks(c).Finish()

	highestVersion, err := semversion.Parse("4.0.7")
	c.Assert(err, tc.ErrorIsNil)
	currentControllerVersion, err := semversion.Parse("4.0.4")
	c.Assert(err, tc.ErrorIsNil)
	oldNodeVersion, err := semversion.Parse("4.0.2")
	c.Assert(err, tc.ErrorIsNil)

	s.agentBinaryFinder.EXPECT().GetHighestPatchVersionAvailableForStream(
		gomock.Any(), modelagent.AgentStreamDevel,
	).Return(highestVersion, nil)
	s.ctrlSt.EXPECT().GetControllerVersion(gomock.Any()).Return(
		currentControllerVersion, nil,
	)
	s.ctrlSt.EXPECT().GetControllerNodeVersions(gomock.Any()).Return(
		map[string]semversion.Number{
			"1": oldNodeVersion,
			"2": currentControllerVersion,
			"3": oldNodeVersion,
		}, nil,
	)
	s.modelSt.EXPECT().GetModelTargetAgentVersion(gomock.Any()).Return(
		currentControllerVersion, nil,
	)

	svc := NewService(s.agentBinaryFinder, s.ctrlSt, s.modelSt)
	_, err = svc.UpgradeControllerWithStream(c.Context(), modelagent.AgentStreamDevel)
	blocker, is := errors.AsType[controllerupgradererrors.ControllerUpgradeBlocker](err)
	c.Check(is, tc.IsTrue)
	c.Check(blocker.Reason, tc.Matches, "controller nodes \\[(3 1|1 3)\\] are not running controller version \"4\\.0\\.4\"")
}

// TestUpgradeControllerWithStream tests the happy path for upgrading a
// controller to the latest available version and also changing the agent stream
// in use.
func (s *serviceSuite) TestUpgradeControllerWithStream(c *tc.C) {
	defer s.setupMocks(c).Finish()

	highestVersion, err := semversion.Parse("4.0.7")
	c.Assert(err, tc.ErrorIsNil)
	currentControllerVersion, err := semversion.Parse("4.0.4")
	c.Assert(err, tc.ErrorIsNil)

	s.agentBinaryFinder.EXPECT().GetHighestPatchVersionAvailableForStream(
		gomock.Any(), modelagent.AgentStreamProposed,
	).Return(highestVersion, nil)
	s.agentBinaryFinder.EXPECT().HasBinariesForVersionAndStream(
		gomock.Any(), highestVersion, modelagent.AgentStreamProposed,
	).Return(true, nil)
	s.ctrlSt.EXPECT().GetControllerVersion(gomock.Any()).Return(
		currentControllerVersion, nil,
	)
	s.ctrlSt.EXPECT().GetControllerNodeVersions(gomock.Any()).Return(
		map[string]semversion.Number{
			"1": currentControllerVersion,
		}, nil,
	)
	s.modelSt.EXPECT().GetModelTargetAgentVersion(gomock.Any()).Return(
		currentControllerVersion, nil,
	)
	s.modelSt.EXPECT().SetModelTargetAgentVersionAndStream(
		gomock.Any(),
		currentControllerVersion,
		highestVersion,
		modelagent.AgentStreamProposed,
	).Return(nil)
	s.ctrlSt.EXPECT().SetControllerVersion(gomock.Any(), highestVersion).Return(nil)

	svc := NewService(s.agentBinaryFinder, s.ctrlSt, s.modelSt)
	upgradedVer, err := svc.UpgradeControllerWithStream(
		c.Context(), modelagent.AgentStreamProposed,
	)
	c.Check(err, tc.ErrorIsNil)
	c.Check(upgradedVer, tc.Equals, highestVersion)
}

// TestUpgradeControllerToVersionZero tests the case where a controller upgrade
// is requested to the zero value of version. In this case, the upgrade must
// fail with the caller getting back an error satisfying [coreerrors.NotValid].
func (s *serviceSuite) TestUpgradeControllerToVersionZero(c *tc.C) {
	defer s.setupMocks(c).Finish()

	svc := NewService(s.agentBinaryFinder, s.ctrlSt, s.modelSt)
	err := svc.UpgradeControllerToVersion(c.Context(), semversion.Zero)
	c.Check(err, tc.ErrorIs, coreerrors.NotValid)
}

// TestUpgradeControllerToVersionDowngrade tests the case where a controller
// upgrade is requested that will result in a downgrade of the current
// controller version. In this case, the upgrade must fail with the caller
// getting back an error satisfying
// [controllerupgradererrors.DowngradeNotSupported].
func (s *serviceSuite) TestUpgradeControllerToVersionDowngrade(c *tc.C) {
	defer s.setupMocks(c).Finish()

	downGradeVersion, err := semversion.Parse("4.0.3")
	c.Assert(err, tc.ErrorIsNil)
	currentControllerVersion, err := semversion.Parse("4.0.8")
	c.Assert(err, tc.ErrorIsNil)

	s.ctrlSt.EXPECT().GetControllerVersion(gomock.Any()).Return(
		currentControllerVersion, nil,
	)
	s.modelSt.EXPECT().GetModelTargetAgentVersion(gomock.Any()).Return(
		currentControllerVersion, nil,
	)

	svc := NewService(s.agentBinaryFinder, s.ctrlSt, s.modelSt)
	err = svc.UpgradeControllerToVersion(c.Context(), downGradeVersion)
	c.Check(err, tc.ErrorIs, controllerupgradererrors.DowngradeNotSupported)
}

// TestUpgradeControllerToVersionNoChange tests the case where a controller
// upgrade is requested for the same version the controller is already running.
// In this case we expect that no short circuiting is done by the service and
// no errors are returned.
//
// The reason for not allowing a short circuit is updating a controller should
// provide the caller an opportunity to make state eventually consistent. i.e
// doing the operation again should fix any inconsistencies in state.
//
// This is also a regression test as the original implementation of the logic
// would error as if a downgrade had been requested.
func (s *serviceSuite) TestUpgradeControllerToVersionNoChange(c *tc.C) {
	defer s.setupMocks(c).Finish()

	upgradeVersion, err := semversion.Parse("4.0.8")
	c.Assert(err, tc.ErrorIsNil)
	currentControllerVersion, err := semversion.Parse("4.0.8")
	c.Assert(err, tc.ErrorIsNil)

	s.ctrlSt.EXPECT().GetControllerVersion(gomock.Any()).Return(
		currentControllerVersion, nil,
	)
	s.modelSt.EXPECT().GetModelTargetAgentVersion(gomock.Any()).Return(
		currentControllerVersion, nil,
	)
	s.agentBinaryFinder.EXPECT().HasBinariesForVersion(
		gomock.Any(), upgradeVersion,
	).Return(true, nil)
	s.ctrlSt.EXPECT().GetControllerNodeVersions(gomock.Any()).Return(
		map[string]semversion.Number{
			"1": currentControllerVersion,
		}, nil,
	)
	s.modelSt.EXPECT().SetModelTargetAgentVersion(
		gomock.Any(), currentControllerVersion, upgradeVersion,
	).Return(nil)
	s.ctrlSt.EXPECT().SetControllerVersion(gomock.Any(), upgradeVersion).
		Return(nil)

	svc := NewService(s.agentBinaryFinder, s.ctrlSt, s.modelSt)
	err = svc.UpgradeControllerToVersion(c.Context(), upgradeVersion)
	c.Check(err, tc.ErrorIsNil)
}

// TestUpgradeControllerToVersionGreaterThanPatch tests the case where a
// controller upgrade is requested to a version that is greater than just a
// patch bump. In this case, the upgrade must fail with the caller getting back
// an error satisfying [controllerupgradererrors.VersionNotSupported].
//
// Controller upgrades are only supported for patch bumps.
func (s *serviceSuite) TestUpgradeControllerToVersionGreaterThanPatch(c *tc.C) {
	defer s.setupMocks(c).Finish()

	upgradeVersion, err := semversion.Parse("4.1.0")
	c.Assert(err, tc.ErrorIsNil)
	currentControllerVersion, err := semversion.Parse("4.0.8")
	c.Assert(err, tc.ErrorIsNil)

	s.ctrlSt.EXPECT().GetControllerVersion(gomock.Any()).Return(
		currentControllerVersion, nil,
	)
	s.modelSt.EXPECT().GetModelTargetAgentVersion(gomock.Any()).Return(
		currentControllerVersion, nil,
	)

	svc := NewService(s.agentBinaryFinder, s.ctrlSt, s.modelSt)
	err = svc.UpgradeControllerToVersion(c.Context(), upgradeVersion)
	c.Check(err, tc.ErrorIs, controllerupgradererrors.VersionNotSupported)
}

// TestUpgradeControllerToVersionMissingBinaries tests the case where a
// controller upgrade is requested to a version that does not have any
// controller binaries available. In this case, the upgrade must fail with the
// caller getting back an error satisfying
// [controllerupgradererrors.MissingControllerBinaries].
func (s *serviceSuite) TestUpgradeControllerToVersionMissingBinaries(c *tc.C) {
	defer s.setupMocks(c).Finish()

	upgradeVersion, err := semversion.Parse("4.0.9")
	c.Assert(err, tc.ErrorIsNil)
	currentControllerVersion, err := semversion.Parse("4.0.8")
	c.Assert(err, tc.ErrorIsNil)

	s.ctrlSt.EXPECT().GetControllerVersion(gomock.Any()).Return(
		currentControllerVersion, nil,
	)
	s.ctrlSt.EXPECT().GetControllerNodeVersions(gomock.Any()).Return(
		map[string]semversion.Number{
			"1": currentControllerVersion,
		}, nil,
	)
	s.modelSt.EXPECT().GetModelTargetAgentVersion(gomock.Any()).Return(
		currentControllerVersion, nil,
	)
	s.agentBinaryFinder.EXPECT().HasBinariesForVersion(
		gomock.Any(), upgradeVersion,
	).Return(false, nil)

	svc := NewService(s.agentBinaryFinder, s.ctrlSt, s.modelSt)
	err = svc.UpgradeControllerToVersion(c.Context(), upgradeVersion)
	c.Check(err, tc.ErrorIs, controllerupgradererrors.MissingControllerBinaries)
}

// TestUpgradeControllerToVersionNodeBlocker tests the case where a controller
// upgrade is requested but one or more controller nodes exist that are not
// running the current controller version. In this case, the upgrade must fail
// with the caller getting back an error satisfying
// [controllerupgradererrors.ControllerUpgradeBlocker].
func (s *serviceSuite) TestUpgradeControllerToVersionNodeBlocker(c *tc.C) {
	defer s.setupMocks(c).Finish()

	upgradeVersion, err := semversion.Parse("4.0.9")
	c.Assert(err, tc.ErrorIsNil)
	currentControllerVersion, err := semversion.Parse("4.0.8")
	c.Assert(err, tc.ErrorIsNil)
	oldNodeVersion, err := semversion.Parse("4.0.0")
	c.Assert(err, tc.ErrorIsNil)

	s.ctrlSt.EXPECT().GetControllerVersion(gomock.Any()).Return(
		currentControllerVersion, nil,
	)
	s.modelSt.EXPECT().GetModelTargetAgentVersion(gomock.Any()).Return(
		currentControllerVersion, nil,
	)
	s.ctrlSt.EXPECT().GetControllerNodeVersions(gomock.Any()).Return(
		map[string]semversion.Number{
			"1": oldNodeVersion,
			"2": oldNodeVersion,
			"3": currentControllerVersion,
		}, nil,
	)

	svc := NewService(s.agentBinaryFinder, s.ctrlSt, s.modelSt)
	err = svc.UpgradeControllerToVersion(c.Context(), upgradeVersion)
	blocker, is := errors.AsType[controllerupgradererrors.ControllerUpgradeBlocker](err)
	c.Check(is, tc.IsTrue)
	c.Check(blocker.Reason, tc.Matches, "controller nodes \\[(2 1|1 2)\\] are not running controller version \"4\\.0\\.8\"")
}

// TestUpgradeControllerToVersionPartialFail tests the case where a controller
// upgrade is performed to a specific version but there is a failure and the
// model and controller databases end up at different versions.
//
// This is a case that could arise where the upgrade can update the model
// database but the fails to update the controller database. This case MUST be
// recoverable by the user. In that they must be able to retry the operation and
// for it to succeed with different versions in the model and controller
// database.
//
// This can work because the model database is the source of truth and updated
// first. The value used in the controller database is just for performing the
// upgrade checks at the moment.
func (s *serviceSuite) TestUpgradeControllerToVersionPartialFail(c *tc.C) {
	defer s.setupMocks(c).Finish()

	upgradeVersion, err := semversion.Parse("4.0.9")
	c.Assert(err, tc.ErrorIsNil)
	currentControllerVersion, err := semversion.Parse("4.0.3")
	c.Assert(err, tc.ErrorIsNil)

	s.agentBinaryFinder.EXPECT().HasBinariesForVersion(
		gomock.Any(), upgradeVersion,
	).Return(true, nil).AnyTimes()

	// Step 1. Setup the failure case where the model write succeeds but the
	// controller write fails.
	s.ctrlSt.EXPECT().GetControllerVersion(gomock.Any()).Return(
		currentControllerVersion, nil,
	)
	// This part is important. We want to show that all controller nodes are
	// running the current controller version.
	s.ctrlSt.EXPECT().GetControllerNodeVersions(gomock.Any()).Return(
		map[string]semversion.Number{
			"1": currentControllerVersion,
			"2": currentControllerVersion,
			"3": currentControllerVersion,
		}, nil,
	)
	s.modelSt.EXPECT().GetModelTargetAgentVersion(gomock.Any()).Return(
		currentControllerVersion, nil,
	)
	s.modelSt.EXPECT().SetModelTargetAgentVersion(
		gomock.Any(), currentControllerVersion, upgradeVersion,
	).Return(nil)
	s.ctrlSt.EXPECT().SetControllerVersion(gomock.Any(), upgradeVersion).Return(
		errors.New("boom"),
	)

	svc := NewService(s.agentBinaryFinder, s.ctrlSt, s.modelSt)
	err = svc.UpgradeControllerToVersion(c.Context(), upgradeVersion)
	c.Check(err, tc.NotNil)

	// Step 2. Change mocks to now report the half saved state and check that a
	// controller upgrade can still be performed.
	s.ctrlSt.EXPECT().GetControllerVersion(gomock.Any()).Return(
		currentControllerVersion, nil,
	)
	// This part is important and shows that given a non deterministic amount of
	// time between a failed call and a retry two controllers have upgrade to
	// the new version. This should not stop the model from completing the
	// upgrade.
	s.ctrlSt.EXPECT().GetControllerNodeVersions(gomock.Any()).Return(
		map[string]semversion.Number{
			"1": currentControllerVersion,
			"2": upgradeVersion,
			"3": upgradeVersion,
		}, nil,
	)
	s.modelSt.EXPECT().GetModelTargetAgentVersion(gomock.Any()).Return(
		upgradeVersion, nil,
	)
	s.modelSt.EXPECT().SetModelTargetAgentVersion(
		gomock.Any(), upgradeVersion, upgradeVersion,
	).Return(nil)
	s.ctrlSt.EXPECT().SetControllerVersion(gomock.Any(), upgradeVersion).Return(
		nil,
	)

	err = svc.UpgradeControllerToVersion(c.Context(), upgradeVersion)
	c.Check(err, tc.ErrorIsNil)
}

// TestUpgradeControllerToVersion is a happy path test for
// [Service.UpgradeControllerToVersion].
func (s *serviceSuite) TestUpgradeControllerToVersion(c *tc.C) {
	defer s.setupMocks(c).Finish()

	upgradeVersion, err := semversion.Parse("4.0.9")
	c.Assert(err, tc.ErrorIsNil)
	currentControllerVersion, err := semversion.Parse("4.0.3")
	c.Assert(err, tc.ErrorIsNil)

	s.agentBinaryFinder.EXPECT().HasBinariesForVersion(
		gomock.Any(), upgradeVersion,
	).Return(true, nil).AnyTimes()

	s.ctrlSt.EXPECT().GetControllerVersion(gomock.Any()).Return(
		currentControllerVersion, nil,
	)
	s.ctrlSt.EXPECT().GetControllerNodeVersions(gomock.Any()).Return(
		map[string]semversion.Number{
			"1": currentControllerVersion,
			"2": currentControllerVersion,
			"3": currentControllerVersion,
		}, nil,
	)
	s.modelSt.EXPECT().GetModelTargetAgentVersion(gomock.Any()).Return(
		currentControllerVersion, nil,
	)
	s.modelSt.EXPECT().SetModelTargetAgentVersion(
		gomock.Any(), currentControllerVersion, upgradeVersion,
	).Return(nil)
	s.ctrlSt.EXPECT().SetControllerVersion(gomock.Any(), upgradeVersion).Return(
		nil,
	)

	svc := NewService(s.agentBinaryFinder, s.ctrlSt, s.modelSt)
	err = svc.UpgradeControllerToVersion(c.Context(), upgradeVersion)
	c.Check(err, tc.ErrorIsNil)
}

// TestUpgradeControllerToVersionAndStreamZero tests the case where a controller
// upgrade is requested to the zero value of version. In this case, the upgrade
// must fail with the caller getting back an error satisfying
// [coreerrors.NotValid].
func (s *serviceSuite) TestUpgradeControllerToVersionAndStreamZero(c *tc.C) {
	defer s.setupMocks(c).Finish()

	svc := NewService(s.agentBinaryFinder, s.ctrlSt, s.modelSt)
	err := svc.UpgradeControllerToVersionAndStream(
		c.Context(), semversion.Zero, modelagent.AgentStreamProposed,
	)
	c.Check(err, tc.ErrorIs, coreerrors.NotValid)
}

// TestUpgradeControllerToVersionAndStreamDowngrade tests the case where a
// controller upgrade is requested that will result in a downgrade of the
// current controller version. In this case, the upgrade must fail with the
// caller getting back an error satisfying
// [controllerupgradererrors.DowngradeNotSupported].
func (s *serviceSuite) TestUpgradeControllerToVersionAndStreamDowngrade(c *tc.C) {
	defer s.setupMocks(c).Finish()

	downGradeVersion, err := semversion.Parse("4.0.3")
	c.Assert(err, tc.ErrorIsNil)
	currentControllerVersion, err := semversion.Parse("4.0.8")
	c.Assert(err, tc.ErrorIsNil)

	s.ctrlSt.EXPECT().GetControllerVersion(gomock.Any()).Return(
		currentControllerVersion, nil,
	)
	s.modelSt.EXPECT().GetModelTargetAgentVersion(gomock.Any()).Return(
		currentControllerVersion, nil,
	)

	svc := NewService(s.agentBinaryFinder, s.ctrlSt, s.modelSt)
	err = svc.UpgradeControllerToVersionAndStream(
		c.Context(), downGradeVersion, modelagent.AgentStreamProposed,
	)
	c.Check(err, tc.ErrorIs, controllerupgradererrors.DowngradeNotSupported)
}

// TestUpgradeControllerToVersionAndStreamNoChange tests the case where a
// controller upgrade is requested for the same version the controller is
// already running. In this case we expect that no short circuiting is done by
// the service and no errors are returned.
//
// The reason for not allowing a short circuit is updating a controller should
// provide the caller an opportunity to make state eventually consistent. i.e
// doing the operation again should fix any inconsistencies in state.
//
// This is also a regression test as the original implementation of the logic
// would error as if a downgrade had been requested.
func (s *serviceSuite) TestUpgradeControllerToVersionAndStreamNoChange(c *tc.C) {
	defer s.setupMocks(c).Finish()

	upgradeVersion, err := semversion.Parse("4.0.8")
	c.Assert(err, tc.ErrorIsNil)
	currentControllerVersion, err := semversion.Parse("4.0.8")
	c.Assert(err, tc.ErrorIsNil)

	s.ctrlSt.EXPECT().GetControllerVersion(gomock.Any()).Return(
		currentControllerVersion, nil,
	)
	s.modelSt.EXPECT().GetModelTargetAgentVersion(gomock.Any()).Return(
		currentControllerVersion, nil,
	)
	s.agentBinaryFinder.EXPECT().HasBinariesForVersionAndStream(
		gomock.Any(), upgradeVersion, modelagent.AgentStreamDevel,
	).Return(true, nil)
	s.ctrlSt.EXPECT().GetControllerNodeVersions(gomock.Any()).Return(
		map[string]semversion.Number{
			"1": currentControllerVersion,
		}, nil,
	)
	s.modelSt.EXPECT().SetModelTargetAgentVersionAndStream(
		gomock.Any(),
		currentControllerVersion,
		upgradeVersion,
		modelagent.AgentStreamDevel,
	).Return(nil)
	s.ctrlSt.EXPECT().SetControllerVersion(gomock.Any(), upgradeVersion).
		Return(nil)

	svc := NewService(s.agentBinaryFinder, s.ctrlSt, s.modelSt)
	err = svc.UpgradeControllerToVersionAndStream(
		c.Context(), upgradeVersion, modelagent.AgentStreamDevel,
	)
	c.Check(err, tc.ErrorIsNil)
}

// TestUpgradeControllerToVersionAndStreamGreaterThanPatch tests the case where
// a controller upgrade is requested to a version that is greater than just a
// patch bump. In this case, the upgrade must fail with the caller getting back
// an error satisfying [controllerupgradererrors.VersionNotSupported].
//
// Controller upgrades are only supported for patch bumps.
func (s *serviceSuite) TestUpgradeControllerToVersionAndStreamGreaterThanPatch(c *tc.C) {
	defer s.setupMocks(c).Finish()

	upgradeVersion, err := semversion.Parse("4.1.0")
	c.Assert(err, tc.ErrorIsNil)
	currentControllerVersion, err := semversion.Parse("4.0.8")
	c.Assert(err, tc.ErrorIsNil)

	s.ctrlSt.EXPECT().GetControllerVersion(gomock.Any()).Return(
		currentControllerVersion, nil,
	)
	s.modelSt.EXPECT().GetModelTargetAgentVersion(gomock.Any()).Return(
		currentControllerVersion, nil,
	)

	svc := NewService(s.agentBinaryFinder, s.ctrlSt, s.modelSt)
	err = svc.UpgradeControllerToVersionAndStream(
		c.Context(), upgradeVersion, modelagent.AgentStreamProposed,
	)
	c.Check(err, tc.ErrorIs, controllerupgradererrors.VersionNotSupported)
}

// TestUpgradeControllerToVersionAndStreamMissingBinaries tests the case where a
// controller upgrade is requested to a version that does not have any
// controller binaries available. In this case, the upgrade must fail with the
// caller getting back an error satisfying
// [controllerupgradererrors.MissingControllerBinaries].
func (s *serviceSuite) TestUpgradeControllerToVersionAndStreamMissingBinaries(c *tc.C) {
	defer s.setupMocks(c).Finish()

	upgradeVersion, err := semversion.Parse("4.0.9")
	c.Assert(err, tc.ErrorIsNil)
	currentControllerVersion, err := semversion.Parse("4.0.8")
	c.Assert(err, tc.ErrorIsNil)

	s.ctrlSt.EXPECT().GetControllerVersion(gomock.Any()).Return(
		currentControllerVersion, nil,
	)
	s.ctrlSt.EXPECT().GetControllerNodeVersions(gomock.Any()).Return(
		map[string]semversion.Number{
			"1": currentControllerVersion,
		}, nil,
	)
	s.modelSt.EXPECT().GetModelTargetAgentVersion(gomock.Any()).Return(
		currentControllerVersion, nil,
	)
	s.agentBinaryFinder.EXPECT().HasBinariesForVersionAndStream(
		gomock.Any(), upgradeVersion, modelagent.AgentStreamProposed,
	).Return(false, nil)

	svc := NewService(s.agentBinaryFinder, s.ctrlSt, s.modelSt)
	err = svc.UpgradeControllerToVersionAndStream(
		c.Context(), upgradeVersion, modelagent.AgentStreamProposed,
	)
	c.Check(err, tc.ErrorIs, controllerupgradererrors.MissingControllerBinaries)
}

// TestUpgradeControllerToVersionAndStreamInvalidStream tests the case where a
// controller upgrade is requested to the latest available version but the
// stream supplied is not valid. In this case, the upgrade must fail with an
// error satisfying [modelagenterrors.AgentStreamNotValid].
func (s *serviceSuite) TestUpgradeControllerToVersionAndStreamInvalidStream(c *tc.C) {
	defer s.setupMocks(c).Finish()

	upgradeVersion, err := semversion.Parse("4.0.9")
	c.Assert(err, tc.ErrorIsNil)

	svc := NewService(s.agentBinaryFinder, s.ctrlSt, s.modelSt)
	err = svc.UpgradeControllerToVersionAndStream(
		c.Context(), upgradeVersion, modelagent.AgentStream(-1),
	)
	c.Check(err, tc.ErrorIs, modelagenterrors.AgentStreamNotValid)
}

// TestUpgradeControllerToVersionAndStreamNodeBlocker tests the case where a
// controller upgrade is requested but one or more controller nodes exist that
// are not running the current controller version. In this case, the upgrade
// must fail with the caller getting back an error satisfying
// [controllerupgradererrors.ControllerUpgradeBlocker].
func (s *serviceSuite) TestUpgradeControllerToVersionAndStreamNodeBlocker(c *tc.C) {
	defer s.setupMocks(c).Finish()

	upgradeVersion, err := semversion.Parse("4.0.9")
	c.Assert(err, tc.ErrorIsNil)
	currentControllerVersion, err := semversion.Parse("4.0.8")
	c.Assert(err, tc.ErrorIsNil)
	oldNodeVersion, err := semversion.Parse("4.0.0")
	c.Assert(err, tc.ErrorIsNil)

	s.ctrlSt.EXPECT().GetControllerVersion(gomock.Any()).Return(
		currentControllerVersion, nil,
	)
	s.modelSt.EXPECT().GetModelTargetAgentVersion(gomock.Any()).Return(
		currentControllerVersion, nil,
	)
	s.ctrlSt.EXPECT().GetControllerNodeVersions(gomock.Any()).Return(
		map[string]semversion.Number{
			"1": oldNodeVersion,
			"2": oldNodeVersion,
			"3": currentControllerVersion,
		}, nil,
	)

	svc := NewService(s.agentBinaryFinder, s.ctrlSt, s.modelSt)
	err = svc.UpgradeControllerToVersionAndStream(
		c.Context(), upgradeVersion, modelagent.AgentStreamProposed,
	)
	blocker, is := errors.AsType[controllerupgradererrors.ControllerUpgradeBlocker](err)
	c.Check(is, tc.IsTrue)
	c.Check(blocker.Reason, tc.Matches, "controller nodes \\[(2 1|1 2)\\] are not running controller version \"4\\.0\\.8\"")
}

// TestUpgradeControllerToVersionAndStreamPartialFail tests the case where a
// controller upgrade is performed to a specific version but there is a failure
// and the model and controller databases end up at different versions.
//
// This is a case that could arise where the upgrade can update the model
// database but the fails to update the controller database. This case MUST be
// recoverable by the user. In that they must be able to retry the operation and
// for it to succeed with different versions in the model and controller
// database.
//
// This can work because the model database is the source of truth and updated
// first. The value used in the controller database is just for performing the
// upgrade checks at the moment.
func (s *serviceSuite) TestUpgradeControllerToVersionStreamPartialFail(c *tc.C) {
	defer s.setupMocks(c).Finish()

	upgradeVersion, err := semversion.Parse("4.0.9")
	c.Assert(err, tc.ErrorIsNil)
	currentControllerVersion, err := semversion.Parse("4.0.3")
	c.Assert(err, tc.ErrorIsNil)

	s.agentBinaryFinder.EXPECT().HasBinariesForVersionAndStream(
		gomock.Any(), upgradeVersion, modelagent.AgentStreamDevel,
	).Return(true, nil).AnyTimes()

	// Step 1. Setup the failure case where the model write succeeds but the
	// controller write fails.
	s.ctrlSt.EXPECT().GetControllerVersion(gomock.Any()).Return(
		currentControllerVersion, nil,
	)
	// This part is important. We want to show that all controller nodes are
	// running the current controller version.
	s.ctrlSt.EXPECT().GetControllerNodeVersions(gomock.Any()).Return(
		map[string]semversion.Number{
			"1": currentControllerVersion,
			"2": currentControllerVersion,
			"3": currentControllerVersion,
		}, nil,
	)
	s.modelSt.EXPECT().GetModelTargetAgentVersion(gomock.Any()).Return(
		currentControllerVersion, nil,
	)
	s.modelSt.EXPECT().SetModelTargetAgentVersionAndStream(
		gomock.Any(),
		currentControllerVersion,
		upgradeVersion,
		modelagent.AgentStreamDevel,
	).Return(nil)
	s.ctrlSt.EXPECT().SetControllerVersion(gomock.Any(), upgradeVersion).Return(
		errors.New("boom"),
	)

	svc := NewService(s.agentBinaryFinder, s.ctrlSt, s.modelSt)
	err = svc.UpgradeControllerToVersionAndStream(
		c.Context(), upgradeVersion, modelagent.AgentStreamDevel,
	)
	c.Check(err, tc.NotNil)

	// Step 2. Change mocks to now report the half saved state and check that a
	// controller upgrade can still be performed.
	s.ctrlSt.EXPECT().GetControllerVersion(gomock.Any()).Return(
		currentControllerVersion, nil,
	)
	// This part is important and shows that given a non deterministic amount of
	// time between a failed call and a retry two controllers have upgrade to
	// the new version. This should not stop the model from completing the
	// upgrade.
	s.ctrlSt.EXPECT().GetControllerNodeVersions(gomock.Any()).Return(
		map[string]semversion.Number{
			"1": currentControllerVersion,
			"2": upgradeVersion,
			"3": upgradeVersion,
		}, nil,
	)
	s.modelSt.EXPECT().GetModelTargetAgentVersion(gomock.Any()).Return(
		upgradeVersion, nil,
	)
	s.modelSt.EXPECT().SetModelTargetAgentVersionAndStream(
		gomock.Any(),
		upgradeVersion,
		upgradeVersion,
		modelagent.AgentStreamDevel,
	).Return(nil)
	s.ctrlSt.EXPECT().SetControllerVersion(gomock.Any(), upgradeVersion).Return(
		nil,
	)

	err = svc.UpgradeControllerToVersionAndStream(
		c.Context(), upgradeVersion, modelagent.AgentStreamDevel,
	)
	c.Check(err, tc.ErrorIsNil)
}

// TestUpgradeControllerToVersionAndStream is a happy path test for
// [Service.UpgradeControllerToVersionAndStream].
func (s *serviceSuite) TestUpgradeControllerToVersionAndStream(c *tc.C) {
	defer s.setupMocks(c).Finish()

	upgradeVersion, err := semversion.Parse("4.0.9")
	c.Assert(err, tc.ErrorIsNil)
	currentControllerVersion, err := semversion.Parse("4.0.3")
	c.Assert(err, tc.ErrorIsNil)

	s.agentBinaryFinder.EXPECT().HasBinariesForVersionAndStream(
		gomock.Any(), upgradeVersion, modelagent.AgentStreamTesting,
	).Return(true, nil).AnyTimes()

	s.ctrlSt.EXPECT().GetControllerVersion(gomock.Any()).Return(
		currentControllerVersion, nil,
	)
	s.ctrlSt.EXPECT().GetControllerNodeVersions(gomock.Any()).Return(
		map[string]semversion.Number{
			"1": currentControllerVersion,
			"2": currentControllerVersion,
			"3": currentControllerVersion,
		}, nil,
	)
	s.modelSt.EXPECT().GetModelTargetAgentVersion(gomock.Any()).Return(
		currentControllerVersion, nil,
	)
	s.modelSt.EXPECT().SetModelTargetAgentVersionAndStream(
		gomock.Any(),
		currentControllerVersion,
		upgradeVersion,
		modelagent.AgentStreamTesting,
	).Return(nil)
	s.ctrlSt.EXPECT().SetControllerVersion(gomock.Any(), upgradeVersion).Return(
		nil,
	)

	svc := NewService(s.agentBinaryFinder, s.ctrlSt, s.modelSt)
	err = svc.UpgradeControllerToVersionAndStream(
		c.Context(), upgradeVersion, modelagent.AgentStreamTesting,
	)
	c.Check(err, tc.ErrorIsNil)
}
