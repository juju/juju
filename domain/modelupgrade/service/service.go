// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	coreerrors "github.com/juju/juju/core/errors"
	corelogger "github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/providertracker"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/domain/modelupgrade"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/upgrades/upgradevalidation"
)

// State describes retrieval and persistence methods for upgrade info.
type State interface {
	// GetModelVersionInfo returns the current model target version
	// and whether the model is the controller model or not.
	// The following errors can be expected:
	// - [modeleerrors.NotFound] when the model does not exist.
	// - [modelerrors.AgentVersionNotFound] when there is no target version found.
	GetModelVersionInfo(ctx context.Context) (semversion.Number, bool, error)
	// SetTargetAgentVersion sets the target agent version and stream for the model.
	SetTargetAgentVersion(ctx context.Context, targetVersion semversion.Number, agentStream *string) error
}

// Service provides the API for performing model upgrades.
type Service struct {
	st     State
	logger corelogger.Logger
}

// JujuUpgradePrechecker mirrors the [environs.JujuUpgradePrechecker] interface.
type JujuUpgradePrechecker interface {
	// PreparePrechecker is called to to give a provider a chance to
	// perform interactive operations that are required for prechecking an upgrade.
	PreparePrechecker(context.Context) error

	// PrecheckUpgradeOperations returns a list of
	// PrecheckJujuUpgradeOperations for checking if juju can be upgrade.
	PrecheckUpgradeOperations() []environs.PrecheckJujuUpgradeOperation
}

// ProviderService provides the API for working with network spaces.
type ProviderService struct {
	Service
	provider func(context.Context) (JujuUpgradePrechecker, error)
}

// NewProviderService returns a new service reference wrapping the input state.
func NewProviderService(
	st State,
	provider providertracker.ProviderGetter[JujuUpgradePrechecker],
	logger corelogger.Logger,
) *ProviderService {
	return &ProviderService{
		Service: Service{
			st:     st,
			logger: logger,
		},
		provider: provider,
	}
}

// UpgradeModel upgrades a model agent version.
// The following errors can be expected:
// - [modelerrors.NotFound] when the model does not exist.
func (s *ProviderService) UpgradeModel(ctx context.Context, arg modelupgrade.UpgradeModelParams) error {
	s.logger.Tracef(ctx, "UpgradeModel arg %#v", arg)

	currentModelVersion, isControllerModel, err := s.st.GetModelVersionInfo(ctx)
	if err != nil {
		return errors.Errorf("getting model current agent version: %w", err)
	}
	// TODO - implement me
	//if model.Life() != Alive {
	//	return modelerrors.NotAlive
	//}

	controllerAgentVersion := arg.ControllerModelVersion
	targetVersion := arg.TargetVersion
	// For non controller models, we use the exact controller
	// model version to upgrade to, unless an explicit target
	// has been specified.
	useControllerVersion := false
	if !isControllerModel {
		if targetVersion == semversion.Zero || targetVersion.Compare(controllerAgentVersion) == 0 {
			targetVersion = controllerAgentVersion
			useControllerVersion = true
		} else if controllerAgentVersion.Compare(targetVersion.ToPatch()) < 0 {
			return errors.Errorf("cannot upgrade to a version %q greater than that of the controller %q", targetVersion, controllerAgentVersion)
		}
	}
	if !useControllerVersion {
		s.logger.Debugf(ctx, "deciding target version for model upgrade, from %q to %q for stream %q", currentModelVersion, targetVersion, arg.AgentStream)
		// TODO - implement me
		// https://github.com/juju/juju/blob/3.6/apiserver/facades/client/modelupgrader/upgrader.go#L206
		// https://github.com/juju/juju/blob/3.6/apiserver/facades/client/modelupgrader/findagents.go#L25
	}

	// Before changing the agent version to trigger an upgrade or downgrade,
	// we'll check the provider.
	provider, err := s.provider(ctx)
	if err != nil && !errors.Is(err, coreerrors.NotSupported) {
		return errors.Errorf("getting provider for model: %w", err)
	}
	if err == nil && isControllerModel {
		if err := preCheckEnvironForUpgradeModel(
			ctx, provider, currentModelVersion, targetVersion, s.logger,
		); err != nil {
			return errors.Errorf("checking provider can be upgraded for model: %w", err)
		}
	}

	if err := s.validateModelUpgrade(ctx, targetVersion); err != nil {
		return errors.Capture(err)
	}
	if !arg.IgnoreAgentVersions {
		// TODO - implement me
		// https://github.com/juju/juju/blob/3.6/state/state.go#L659
	}
	if arg.DryRun {
		return nil
	}

	var agentStream *string
	if arg.AgentStream != "" {
		agentStream = &arg.AgentStream
	}
	if err := s.st.SetTargetAgentVersion(ctx, targetVersion, agentStream); err != nil {
		return errors.Errorf("setting model %q agent version: %w", targetVersion, err)
	}
	return nil
}

func preCheckEnvironForUpgradeModel(
	ctx context.Context,
	provider JujuUpgradePrechecker,
	currentVersion, targetVersion semversion.Number,
	logger corelogger.Logger,
) error {
	// skipTarget returns true if the from version is less than the target version
	// AND the target version is greater than the to version.
	// Borrowed from upgrades.opsIterator.
	skipTarget := func(from, target, to semversion.Number) bool {
		// Clear the version tag of the to release to ensure that all
		// upgrade steps for the release are run for alpha and beta
		// releases.
		// ...but only do this if the from version has actually changed,
		// lest we trigger upgrade mode unnecessarily for non-final
		// versions.
		if from.Compare(to) != 0 {
			to.Tag = ""
		}
		// Do not run steps for versions of Juju earlier or same as we are upgrading from.
		if target.Compare(from) <= 0 {
			return true
		}
		// Do not run steps for versions of Juju later than we are upgrading to.
		if target.Compare(to) > 0 {
			return true
		}
		return false
	}

	if err := provider.PreparePrechecker(ctx); err != nil {
		return err
	}

	for _, op := range provider.PrecheckUpgradeOperations() {
		if skipTarget(currentVersion, op.TargetVersion, targetVersion) {
			logger.Debugf(ctx, "ignoring precheck upgrade operation for version %s", op.TargetVersion)
			continue
		}
		logger.Debugf(ctx, "running precheck upgrade operation for version %s", op.TargetVersion)
		for _, step := range op.Steps {
			logger.Debugf(ctx, "running precheck step %q", step.Description())
			if err := step.Run(); err != nil {
				return errors.Errorf("Unable to upgrade to %s: %w", targetVersion, err)
			}
		}
	}
	return nil
}

func (s *Service) validateModelUpgrade(ctx context.Context, targetVersion semversion.Number) (err error) {
	var blockers *upgradevalidation.ModelUpgradeBlockers
	defer func() {
		if err == nil && blockers != nil {
			err = errors.Errorf(
				"cannot upgrade to %q due to issues with these models:\n%s",
				targetVersion, blockers,
			).Add(coreerrors.NotSupported)
		}
	}()

	// TODO - implement me
	// https://github.com/juju/juju/blob/3.6/apiserver/facades/client/modelupgrader/upgrader.go#L228
	return nil
}
