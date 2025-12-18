// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"fmt"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/core/trace"
	"github.com/juju/juju/domain/agentbinary"
	controllerupgradererrors "github.com/juju/juju/domain/controllerupgrader/errors"
	modelagenterrors "github.com/juju/juju/domain/modelagent/errors"
	"github.com/juju/juju/internal/errors"
)

// AgentBinaryFinder defines a agentFinder for asserting if agent binaries are
// available and identifying upgrade versions.
type AgentBinaryFinder interface {
	// HasBinariesForVersionAndArchitectures will interrogate agent binaries available in the
	// system and return true or false if agent binaries exist for the provided
	// version and architectures.
	HasBinariesForVersionAndArchitectures(
		context.Context,
		semversion.Number,
		[]agentbinary.Architecture,
	) (bool, error)

	// HasBinariesForVersionStreamAndArchitectures will interrogate agent binaries available
	// in the system and return true or false if agent binaries exist for the
	// provided version and architectures on the supplied stream.
	HasBinariesForVersionStreamAndArchitectures(
		context.Context,
		semversion.Number,
		agentbinary.Stream,
		[]agentbinary.Architecture,
	) (bool, error)

	// GetHighestPatchVersionAvailable will return the highest available patch
	// version available for the current controller version.
	GetHighestPatchVersionAvailable(context.Context) (semversion.Number, error)

	// GetHighestPatchVersionAvailableForStream will return the highest available patch
	// version available for the current controller version and stream.
	GetHighestPatchVersionAvailableForStream(
		context.Context, agentbinary.Stream,
	) (semversion.Number, error)
}

// ControllerState defines the interface for interacting with the underlying
// controller version.
type ControllerState interface {
	// GetControllerNodeVersions returns the current version that is running for
	// each controller in the cluster. This is the version that each controller
	// reports when it starts up.
	GetControllerNodeVersions(ctx context.Context) (map[string]semversion.Number, error)

	// GetControllerTargetVersion returns the target controller version in use by the
	// cluster.
	GetControllerTargetVersion(ctx context.Context) (semversion.Number, error)

	// SetControllerTargetVersion is responsible for setting the current
	// controller version in use by the cluster. Controllers in the cluster will
	// eventually upgrade to this version once changed.
	SetControllerTargetVersion(context.Context, semversion.Number) error
}

// ControllerModelState defines the interface for interacting with the
// underlying model that hosts the current controller(s). Model state is
// required for the controller upgrader so that the target agent version of the
// model can be upgraded in lock step with the controller version.
type ControllerModelState interface {
	// GetModelTargetAgentVersion returns the target agent version currently set
	// for the controller's model.
	GetModelTargetAgentVersion(context.Context) (semversion.Number, error)

	// SetModelTargetAgentVersion is responsible for setting the current target
	// agent version of the controller model. This function expects a
	// precondition version to be supplied. The model's target version at the
	// time the operation is applied must match the preCondition version or else
	// an error is returned.
	SetModelTargetAgentVersion(
		ctx context.Context,
		preCondition semversion.Number,
		toVersion semversion.Number,
	) error

	// SetModelTargetAgentVersionAndStream is responsible for setting the
	// current target agent version of the controller model and the agent stream
	// that is used. This function expects a precondition version to be supplied.
	// The model's target version at the time the operation is applied must
	// match the preCondition version or else an error is returned.
	SetModelTargetAgentVersionAndStream(
		ctx context.Context,
		preCondition semversion.Number,
		toVersion semversion.Number,
		stream agentbinary.Stream,
	) error
}

// Service defines a service for interacting with the controller's version and
// performing upgrades to the cluster.
type Service struct {
	agentBinaryFinder AgentBinaryFinder
	ctrlSt            ControllerState
	modelSt           ControllerModelState
}

// NewService returns a new Service for interacting and upgrading the
// controller's version.
func NewService(
	agentBinaryFinder AgentBinaryFinder,
	ctrlSt ControllerState,
	modelSt ControllerModelState,
) *Service {
	return &Service{
		agentBinaryFinder: agentBinaryFinder,
		ctrlSt:            ctrlSt,
		modelSt:           modelSt,
	}
}

// UpgradeController upgrades the current clusters set of controllers to the
// latest Juju version available. The controller(s) will only be upgraded to new
// patch versions within the current major and minor version.
//
// The following errors may be expected:
// - [controllerupgradererrors.ControllerUpgradeBlocker] describing a block that
// exists preventing a controller upgrade from proceeding.
func (s *Service) UpgradeController(
	ctx context.Context,
) (semversion.Number, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	desiredVersion, err := s.agentBinaryFinder.GetHighestPatchVersionAvailable(ctx)
	if err != nil {
		return semversion.Zero, errors.Errorf(
			"getting desired controller version to upgrade to: %w", err,
		)
	}

	// NOTE (tlm): Because this func uses
	// [Service.UpgradeControllerToVersion] to compose its implementation. This
	// func must handle the contract of UpgradeControllerToVersion. Specifically
	// the errors returned don't align with the expectations of the caller. The
	// below switch statement re-writes the error cases to better explain the
	// very unlikely error that has occurred. These exists to point a developer
	// at the problem and not to offer any actionable item for a caller.
	err = s.UpgradeControllerToVersion(ctx, desiredVersion)

	switch {
	case errors.Is(err, controllerupgradererrors.DowngradeNotSupported):
		return semversion.Zero, errors.Errorf(
			"upgrading controller to recommended version %q is considered a downgrade",
			desiredVersion,
		)
	case errors.Is(err, controllerupgradererrors.VersionNotSupported):
		return semversion.Zero, errors.Errorf(
			"upgrading controller to recommended version %q is not supported",
			desiredVersion,
		)
	case errors.Is(err, controllerupgradererrors.MissingControllerBinaries):
		return semversion.Zero, errors.Errorf(
			"updating controller to recommended version %q is missing agent binaries",
			desiredVersion,
		)
	case err != nil:
		return semversion.Zero, errors.Errorf(
			"upgrading controller to recommended version %q: %w",
			desiredVersion, err,
		)
	}

	return desiredVersion, nil
}

// UpgradeControllerWithStream upgrades the current clusters set of controllers
// to the latest Juju version available. Also changed is the agent stream used
// for controller binaries. The controller will only be upgraded to new patch
// versions within the current major and minor version.
//
// The following errors may be expected:
// - [modelagenterrors.AgentStreamNotValid] when the agent stream being upgraded
// to is not valid.
// - [controllerupgradererrors.ControllerUpgradeBlocker] describing a block that
// exists preventing a controller upgrade from proceeding.
func (s *Service) UpgradeControllerWithStream(
	ctx context.Context,
	stream agentbinary.Stream,
) (semversion.Number, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if !stream.IsValid() {
		return semversion.Zero, errors.New(
			"agent stream is not valid",
		).Add(modelagenterrors.AgentStreamNotValid)
	}

	desiredVersion, err := s.agentBinaryFinder.GetHighestPatchVersionAvailableForStream(
		ctx, stream,
	)
	if err != nil {
		return semversion.Zero, errors.Errorf(
			"getting desired controller version to upgrade to: %w", err,
		)
	}

	// NOTE (tlm): Because this func uses
	// [Service.UpgradeControllerToVersionWithStream] to compose its
	// implementation. This func must handle the contract of
	// UpgradeControllerToVersionWithStream. Specifically the errors returned
	// don't align with the expectations of the caller. The
	// below switch statement re-writes the error cases to better explain the
	// very unlikely error that has occurred. These exists to point a developer
	// at the problem and not to offer any actionable item for a caller.
	err = s.UpgradeControllerToVersionWithStream(ctx, desiredVersion, stream)

	switch {
	case errors.Is(err, modelagenterrors.AgentStreamNotValid):
		return semversion.Zero, err
	case errors.Is(err, controllerupgradererrors.DowngradeNotSupported):
		return semversion.Zero, errors.Errorf(
			"upgrading controller to recommended version %q is considered a downgrade",
			desiredVersion,
		)
	case errors.Is(err, controllerupgradererrors.VersionNotSupported):
		return semversion.Zero, errors.Errorf(
			"upgrading controller to recommended version %q is not supported",
			desiredVersion,
		)
	case errors.Is(err, controllerupgradererrors.MissingControllerBinaries):
		return semversion.Zero, errors.Errorf(
			"updating controller to recommended version %q is missing agent binaries",
			desiredVersion,
		)
	case err != nil:
		return semversion.Zero, errors.Errorf(
			"upgrading controller to recommended version %q: %w",
			desiredVersion, err,
		)
	}

	return desiredVersion, nil
}

// UpgradeControllerToVersion upgrades the current clusters set of controllers
// to the specified version. All controllers participating in the cluster will
// eventually be upgraded to the new version after this call successfully
// returns.
//
// The version supplied as part of this upgrade must not be a downgrade and must
// not upgrade either the major or minor part of the current version. Only
// patch upgrades are permissible for controller upgrades.
//
// The following errors may be expected:
// - [coreerrors.NotValid] if the upgrade to version supplied is not a valid
// version number.
// - [controllerupgradererrors.DowngradeNotSupported] if the requested version
// being upgraded to would result in a downgrade of the controller.
// - [controllerupgradererrors.VersionNotSupported] if the requested version
// being upgraded to is more than a patch version upgrade.
// - [controllerupgradererrors.MissingControllerBinaries] if no controller
// binaries can be found for the requested version.
// - [controllerupgradererrors.ControllerUpgradeBlocker] describing a block that
// exists preventing a controller upgrade from proceeding.
func (s *Service) UpgradeControllerToVersion(
	ctx context.Context,
	desiredVersion semversion.Number,
) error {
	// Controller upgrades are still controlled by that of the model agent
	// version for the controllers model. Under the covers this is how they
	// still work.
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	err := s.runPreUpgradeChecksToVersion(ctx, desiredVersion)
	if err != nil {
		return err
	}

	modelTargetAgentVersion, err := s.modelSt.GetModelTargetAgentVersion(ctx)
	if err != nil {
		return errors.Errorf(
			"getting controller model agent version: %w", err,
		)
	}

	// Both controller and model upgrading is always driven off of the target
	// agent version of the model. Because of this we always do the set
	// operation on the model first. If setting the controller version in the
	// controller database fails it will not be the end of the world.
	//
	// If the order of sets gets reversed then we can end up in a state where no
	// upgrade to the controller may be performed at all.
	err = s.modelSt.SetModelTargetAgentVersion(
		ctx, modelTargetAgentVersion, desiredVersion,
	)
	if err != nil {
		return errors.Errorf(
			"upgrading target agent version for controller model: %w", err,
		)
	}

	err = s.ctrlSt.SetControllerTargetVersion(ctx, desiredVersion)
	if err != nil {
		return errors.Capture(err)
	}
	return nil
}

// UpgradeControllerToVersionWithStream upgrades the current clusters set of
// controllers to the specified version. Also updated is the agent stream used
// for getting the controller binaries. All controllers participating in the
// cluster will eventually be upgraded to the new version after this call
// successfully returns.
//
// The version supplied as part of this upgrade must not be a downgrade and must
// not upgrade either the major or minor part of the current version. Only
// patch upgrades are permissible for controller upgrades.
//
// The following errors may be expected:
// - [coreerrors.NotValid] if the upgrade to version supplied is not a valid
// version number.
// - [controllerupgradererrors.DowngradeNotSupported] if the requested version
// being upgraded to would result in a downgrade of the controller.
// - [controllerupgradererrors.VersionNotSupported] if the requested version
// being upgraded to is more than a patch version upgrade.
// - [controllerupgradererrors.MissingControllerBinaries] if no controller
// binaries can be found for the requested version.
// - [modelagenterrors.AgentStreamNotValid] when the agent stream being upgraded
// to is not valid.
// - [controllerupgradererrors.ControllerUpgradeBlocker] describing a block that
// exists preventing a controller upgrade from proceeding.
func (s *Service) UpgradeControllerToVersionWithStream(
	ctx context.Context,
	desiredVersion semversion.Number,
	stream agentbinary.Stream,
) error {
	// Controller upgrades are still controlled by that of the model agent
	// version for the controllers model. Under the covers this is how they
	// still work.
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	err := s.RunPreUpgradeChecksToVersionWithStream(ctx, desiredVersion, stream)
	if err != nil {
		return err
	}

	modelTargetAgentVersion, err := s.modelSt.GetModelTargetAgentVersion(ctx)
	if err != nil {
		return errors.Errorf(
			"getting controller model agent version: %w", err,
		)
	}

	// Both controller and model upgrading is always driven off of the target
	// agent version of the model. Because of this we always do the set
	// operation on the model first. If setting the controller version in the
	// controller database fails it will not be the end of the world.
	//
	// If the order of sets gets reversed then we can end up in a state where no
	// upgrade to the controller may be performed at all.
	err = s.modelSt.SetModelTargetAgentVersionAndStream(
		ctx, modelTargetAgentVersion, desiredVersion, stream,
	)
	if err != nil {
		return errors.Errorf(
			"upgrading target agent version for controller model: %w", err,
		)
	}

	err = s.ctrlSt.SetControllerTargetVersion(ctx, desiredVersion)
	if err != nil {
		return errors.Capture(err)
	}
	return nil
}

// validateControllerCanBeUpgrade is a set of validation checks run on all
// controller(s) in the cluster to make sure they are in a state suitable
// for upgrading.
//
// Specifically as part of this validation we check that each controller is
// running the expected version. If not it means that a previous upgrade is
// still ongoing.
//
// The checks performed are not guaranteed to be valid after this func returns.
//
// The following errors may be expected:
// - [controllerupgradererrors.ControllerUpgradeBlocker] describing a block that
// exists preventing a controller upgrade from proceeding.
func (s *Service) validateControllerCanBeUpgraded(
	ctx context.Context,
	currentVersion semversion.Number,
) error {
	controllerNodeVersions, err := s.ctrlSt.GetControllerNodeVersions(ctx)
	if err != nil {
		return errors.Errorf(
			"getting controller version for each node in cluster: %w", err,
		)
	}

	// blockedNodes is the set of nodes that are blocking the controller
	// upgrade.
	blockedNodes := []string{}
	for node, version := range controllerNodeVersions {
		// if the current version for controllers is greater than that of this
		// node, then this node still needs to be upgraded.
		//
		// It is on purpose that this is not an exact check. Due to the
		// possibility the model agent version could be out of sync with the
		// controller version it is possible for the controller nodes to be
		// running a version higher than the current version. This is ok and
		// permissible.
		if currentVersion.Compare(version) > 0 {
			blockedNodes = append(blockedNodes, node)
		}
	}

	if len(blockedNodes) > 0 {
		return controllerupgradererrors.ControllerUpgradeBlocker{
			Reason: fmt.Sprintf(
				"controller nodes %v are not running controller version %q",
				blockedNodes, currentVersion,
			),
		}
	}

	return nil
}

// validateControllerCanBeUpgradedTo checks that the controller can be upgraded
// from the current version to the new desired version.
//
// The following errors may be expected:
// - [controllerupgradererrors.DowngradeNotSupported] if the requested version
// being upgraded to would result in a downgrade of the controller.
// - [controllerupgradererrors.VersionNotSupported] if the requested version
// being upgraded to is more than a patch version upgrade.
// - [controllerupgradererrors.ControllerUpgradeBlocker] describing a block that
// exists preventing a controller upgrade from proceeding.
func (s *Service) validateControllerCanBeUpgradedTo(
	ctx context.Context,
	desiredVersion semversion.Number,
) error {
	currentVersion, err := s.ctrlSt.GetControllerTargetVersion(ctx)
	if err != nil {
		return errors.Errorf(
			"getting current controller version: %w", err,
		)
	}

	// Check that a downgrade is not being attempted.
	if currentVersion.Compare(desiredVersion) > 0 {
		return errors.New(
			"controller version downgrades are not supported",
		).Add(controllerupgradererrors.DowngradeNotSupported)
	}

	if desiredVersion.Major != currentVersion.Major ||
		desiredVersion.Minor != currentVersion.Minor {
		return errors.New(
			"controller version upgrades are only supported for patch versions",
		).Add(controllerupgradererrors.VersionNotSupported)
	}

	err = s.validateControllerCanBeUpgraded(ctx, currentVersion)
	if err != nil {
		return errors.Capture(err)
	}

	return nil
}

// RunPreUpgradeChecks determines whether the controller can be upgraded
// to the latest available patch version. It returns the desired version
// that the controller can upgrade to if all validation checks pass. The
// method ensures that the desired version can be retrieved and is valid,
// that the upgrade does not attempt a downgrade or a cross-minor or major
// version change, that all controller nodes are synchronized and not
// blocking the upgrade, and that controller binaries exist for the desired
// version and architecture.
//
// The following errors may be expected:
// - [controllerupgradererrors.DowngradeNotSupported] if the requested version
// being upgraded to would result in a downgrade of the controller.
// - [controllerupgradererrors.VersionNotSupported] if the requested version
// being upgraded to is more than a patch version upgrade.
// - [controllerupgradererrors.ControllerUpgradeBlocker] describing a block that
// exists preventing a controller upgrade from proceeding.
func (s *Service) RunPreUpgradeChecks(ctx context.Context) (semversion.Number, error) {
	desiredVersion, err := s.agentBinaryFinder.GetHighestPatchVersionAvailable(ctx)
	if err != nil {
		return semversion.Zero, errors.Errorf(
			"getting desired controller version to upgrade to: %w", err,
		)
	}
	err = s.runPreUpgradeChecksToVersion(ctx, desiredVersion)
	if err != nil {
		return semversion.Zero, err
	}

	return desiredVersion, nil
}

// runPreUpgradeChecksToVersion determines whether the controller can be safely
// upgraded to the specified version. It performs validation checks to ensure
// that the target version is valid and that the upgrade can proceed. The method
// ensures that the desired version is not the zero value, that the upgrade does
// not attempt a downgrade or a non-patch version change, that all controller
// nodes are in a consistent state and not blocking the upgrade, and that
// controller binaries exist for the specified version and required
// architectures.
//
// The following errors may be expected: -
// [controllerupgradererrors.DowngradeNotSupported] if the requested version
// being upgraded to would result in a downgrade of the controller. -
// [controllerupgradererrors.VersionNotSupported] if the requested version being
// upgraded to is more than a patch version upgrade. -
// [controllerupgradererrors.ControllerUpgradeBlocker] describing a block that
// exists preventing a controller upgrade from proceeding.
func (s *Service) runPreUpgradeChecksToVersion(ctx context.Context, desiredVersion semversion.Number) error {
	// We should not continue any further if the version is a zero value.
	if desiredVersion == semversion.Zero {
		return errors.New(
			"controller version cannot be zero",
		).Add(coreerrors.NotValid)
	}

	err := s.validateControllerCanBeUpgradedTo(ctx, desiredVersion)
	if err != nil {
		return errors.Capture(err)
	}

	// TODO(adisazhar123): Refactor this. Ideally we will get the architecture off of the
	// controller and model state. We hardcode this for now.
	architectures := []agentbinary.Architecture{agentbinary.AMD64}
	hasBinaries, err := s.agentBinaryFinder.HasBinariesForVersionAndArchitectures(
		ctx, desiredVersion, architectures,
	)
	if err != nil {
		return errors.Errorf(
			"checking if binaries exist for version %q: %w", desiredVersion, err,
		)
	}
	if !hasBinaries {
		return errors.Errorf(
			"no controller binaries exist for version %q", desiredVersion,
		).Add(controllerupgradererrors.MissingControllerBinaries)
	}

	return nil
}

// RunPreUpgradeChecksWithStream determines whether the controller can be
// upgraded to the latest available patch version within the specified agent
// stream. It returns the desired version that the controller can upgrade to if
// all validation checks pass.
//
// The method first ensures that the provided agent stream is valid, then
// retrieves the highest available patch version for that stream. It validates
// that upgrading to the retrieved version is supported and safe, and confirms
// that the required controller binaries exist for the version, stream, and
// architectures.
//
// The following errors may be expected: -
// [controllerupgradererrors.DowngradeNotSupported] if the requested version
// being upgraded to would result in a downgrade of the controller. -
// [controllerupgradererrors.VersionNotSupported] if the requested version being
// upgraded to is more than a patch version upgrade. -
// [controllerupgradererrors.ControllerUpgradeBlocker] describing a block that
// exists preventing a controller upgrade from proceeding.
func (s *Service) RunPreUpgradeChecksWithStream(ctx context.Context, stream agentbinary.Stream) (semversion.Number, error) {
	if !stream.IsValid() {
		return semversion.Zero, errors.New(
			"agent stream is not valid",
		).Add(modelagenterrors.AgentStreamNotValid)
	}

	desiredVersion, err := s.agentBinaryFinder.GetHighestPatchVersionAvailableForStream(
		ctx, stream,
	)
	if err != nil {
		return semversion.Zero, errors.Errorf(
			"getting desired controller version to upgrade to: %w", err,
		)
	}

	err = s.RunPreUpgradeChecksToVersionWithStream(ctx, desiredVersion, stream)
	if err != nil {
		return semversion.Zero, errors.Capture(err)
	}

	return desiredVersion, nil
}

// RunPreUpgradeChecksToVersionWithStream determines whether the controller can
// be safely upgraded to the specified version within the given agent stream. It
// verifies that the desired version and stream are valid, ensures the upgrade
// does not attempt a downgrade or a non-patch upgrade, checks that all
// controller nodes are in a consistent state, and confirms that the necessary
// controller binaries exist for the specified version, stream, and
// architectures.
//
// The following errors may be expected: -
// [controllerupgradererrors.DowngradeNotSupported] if the requested version
// being upgraded to would result in a downgrade of the controller. -
// [controllerupgradererrors.VersionNotSupported] if the requested version being
// upgraded to is more than a patch version upgrade. -
// [controllerupgradererrors.ControllerUpgradeBlocker] describing a block that
// exists preventing a controller upgrade from proceeding.
func (s *Service) RunPreUpgradeChecksToVersionWithStream(ctx context.Context, desiredVersion semversion.Number, stream agentbinary.Stream) error {
	// We should not continue any further if the version is a zero value.
	if desiredVersion == semversion.Zero {
		return errors.New(
			"controller version cannot be zero",
		).Add(coreerrors.NotValid)
	}

	if !stream.IsValid() {
		return errors.New(
			"agent stream is not valid",
		).Add(modelagenterrors.AgentStreamNotValid)
	}

	err := s.validateControllerCanBeUpgradedTo(ctx, desiredVersion)
	if err != nil {
		return errors.Capture(err)
	}

	// TODO(adisazhar123): Refactor this. Ideally we will get the architecture off of the
	// controller and model state. We hardcode this for now.
	architectures := []agentbinary.Architecture{agentbinary.AMD64}
	hasBinaries, err := s.agentBinaryFinder.HasBinariesForVersionStreamAndArchitectures(
		ctx, desiredVersion, stream, architectures,
	)
	if err != nil {
		return errors.Errorf(
			"checking if binaries exist for version %q: %w", desiredVersion, err,
		)
	}
	if !hasBinaries {
		return errors.Errorf(
			"no controller binaries exist for version %q", desiredVersion,
		).Add(controllerupgradererrors.MissingControllerBinaries)
	}

	return nil
}
