// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelupgrader

import (
	"context"

	"github.com/juju/names/v6"

	"github.com/juju/juju/apiserver/authentication"
	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/core/agentbinary"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/semversion"
	controllerupgradererrors "github.com/juju/juju/domain/controllerupgrader/errors"
	"github.com/juju/juju/domain/modelagent"
	modelagenterrors "github.com/juju/juju/domain/modelagent/errors"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/rpc/params"
)

// ControllerUpgraderService mirrors the controller upgrader service
// so we can easily mock it for unit tests.
type ControllerUpgraderService interface {
	// UpgradeController will upgrade the current clusters set of controllers to the latest
	// patch version within the current controller's major and minor version.
	UpgradeController(ctx context.Context) (semversion.Number, error)
	// UpgradeControllerWithStream upgrades the current clusters set of controllers
	// to the latest Juju version available using the given stream.
	// It will be updated latest to the latest patch version within the current
	// controller's major and minor version.
	UpgradeControllerWithStream(
		ctx context.Context,
		stream modelagent.AgentStream,
	) (semversion.Number, error)
	// UpgradeControllerToVersion upgrades the current clusters set of controllers
	// to the specified version.
	UpgradeControllerToVersion(
		ctx context.Context,
		desiredVersion semversion.Number,
	) error
	// UpgradeControllerToVersionAndStream upgrades the current clusters set of
	// controllers to the specified version. It will grab the binaries that is based
	// on the given stream.
	UpgradeControllerToVersionAndStream(
		ctx context.Context,
		desiredVersion semversion.Number,
		stream modelagent.AgentStream,
	) error

	// CanUpgradeController determines whether the controller can be upgraded
	// to the latest available patch version.
	CanUpgradeController(ctx context.Context) (semversion.Number, error)

	// CanUpgradeControllerToVersion determines whether the controller can be safely
	// upgraded to the specified version. It performs validation checks to ensure that
	// the target version is valid and that the upgrade can proceed.
	CanUpgradeControllerToVersion(ctx context.Context, desiredVersion semversion.Number) error

	// CanUpgradeControllerWithStream determines whether the controller can be upgraded
	// to the latest available patch version within the specified agent stream. It returns
	// the desired version that the controller can upgrade to if all validation checks pass.
	CanUpgradeControllerWithStream(ctx context.Context, stream modelagent.AgentStream) (semversion.Number, error)

	// CanUpgradeControllerToVersionWithStream determines whether the controller can be
	// safely upgraded to the specified version within the given agent stream.
	CanUpgradeControllerToVersionWithStream(ctx context.Context, desiredVersion semversion.Number, stream modelagent.AgentStream) error
}

// ControllerUpgraderAPI upgrades a controller and a model hosting the controller.
type ControllerUpgraderAPI struct {
	authorizer facade.Authorizer
	check      common.BlockCheckerInterface

	upgraderService ControllerUpgraderService

	controllerTag names.Tag
	modelTag      names.Tag
}

// NewControllerUpgraderAPI instantiates a new [ControllerUpgraderAPI].
func NewControllerUpgraderAPI(
	controllerTag names.Tag,
	modelTag names.Tag,
	authorizer facade.Authorizer,
	check common.BlockCheckerInterface,
	upgraderService ControllerUpgraderService,
) *ControllerUpgraderAPI {
	return &ControllerUpgraderAPI{
		controllerTag:   controllerTag,
		modelTag:        modelTag,
		authorizer:      authorizer,
		check:           check,
		upgraderService: upgraderService,
	}
}

// AbortModelUpgrade returns not supported, as it's not possible to move
// back to a prior version.
func (c *ControllerUpgraderAPI) AbortModelUpgrade(_ context.Context, _ params.ModelParam) error {
	return errors.New("aborting model upgrades is not supported").Add(coreerrors.NotSupported)
}

// canUpgrade has the responsibility to determine whether there is sufficient permission
// to perform an upgrade.
func (c *ControllerUpgraderAPI) canUpgrade(ctx context.Context, model names.ModelTag) (bool, error) {
	if model.Id() != c.modelTag.Id() {
		return false, nil
	}
	err := c.authorizer.HasPermission(
		ctx,
		permission.SuperuserAccess,
		c.controllerTag,
	)
	if err != nil && !errors.Is(err, authentication.ErrorEntityMissingPermission) {
		return false, errors.Capture(err)
	}
	if err == nil {
		return true, nil
	}

	err = c.authorizer.HasPermission(
		ctx,
		permission.WriteAccess,
		model,
	)
	if err != nil {
		if errors.Is(err, authentication.ErrorEntityMissingPermission) {
			return false, nil
		}
		return false, errors.Capture(err)
	}
	return true, nil
}

func (c *ControllerUpgraderAPI) mapResponse(err error, targetVersion semversion.Number, arg params.UpgradeModelParams) (params.UpgradeModelResult, error) {
	var result params.UpgradeModelResult
	// Map the errors to respect what the existing API returns.
	// We mirror as closely as possible to [UpgradeModel] func in [modelupgrader.ModelUpgraderAPI].
	switch {
	case errors.Is(err, controllerupgradererrors.MissingControllerBinaries):
		result.Error = &params.Error{
			Message: err.Error(),
			Code:    params.CodeNotFound,
		}
		return result, nil
	case errors.HasType[controllerupgradererrors.ControllerUpgradeBlocker](err):
		message := err.Error()
		err, ok := errors.AsType[controllerupgradererrors.ControllerUpgradeBlocker](err)
		if ok {
			message = err.Reason
		}
		result.Error = &params.Error{
			Message: message,
			Code:    params.CodeNotSupported,
		}
		return result, nil
	case errors.Is(err, controllerupgradererrors.VersionNotSupported):
		e := errors.Errorf(
			"cannot upgrade to a version %q greather than that of the controller", targetVersion,
		).Add(coreerrors.NotValid)
		return result, errors.Capture(e)
	case errors.Is(err, modelagenterrors.AgentStreamNotValid):
		e := errors.Errorf(
			"agent stream %q is not recognised as a valid value", arg.AgentStream,
		).Add(coreerrors.NotValid)
		return result, errors.Capture(e)
	case err != nil:
		return result, errors.Capture(apiservererrors.ServerError(err))
	}

	result.ChosenVersion = targetVersion
	return result, nil
}

// doUpgrade has the responsibility of delegating the upgrade to the service. It determines which func to invoke
// by interrogating the values set in [params.UpgradeModelParams].
// A post-processing step is performed to map the errors returned from the service to ones the existing API
// conforms to.
func (c *ControllerUpgraderAPI) doUpgrade(ctx context.Context, arg params.UpgradeModelParams) (params.UpgradeModelResult, error) {
	var (
		hasStreamChange  = arg.AgentStream != ""
		hasTargetVersion = arg.TargetVersion != semversion.Zero
		targetStream     modelagent.AgentStream
		targetVersion    = arg.TargetVersion
		upgrader         func(context.Context) error
		dryRunValidate   func(context.Context) (semversion.Number, error)
		result           params.UpgradeModelResult
		err              error
	)

	// Parse the agent stream.
	if arg.AgentStream != "" {
		targetStream, err = modelagent.AgentStreamFromCoreAgentStream(agentbinary.AgentStream(arg.AgentStream))
		if err != nil {
			if errors.Is(err, coreerrors.NotValid) {
				return result, errors.Capture(errors.Errorf(
					"agent stream %q is not recognised as a valid value", arg.AgentStream,
				).Add(coreerrors.NotValid))
			}
			return result, errors.Capture(err)
		}
	}

	// Delegate it to the service depending on what arguments
	// are supplied.
	switch {
	case hasTargetVersion && hasStreamChange:
		upgrader = func(ctx context.Context) error {
			return c.upgraderService.UpgradeControllerToVersionAndStream(ctx, targetVersion, targetStream)
		}
		dryRunValidate = func(ctx context.Context) (semversion.Number, error) {
			return targetVersion, c.upgraderService.CanUpgradeControllerToVersionWithStream(ctx, targetVersion, targetStream)
		}
	case hasTargetVersion && !hasStreamChange:
		upgrader = func(ctx context.Context) error {
			return c.upgraderService.UpgradeControllerToVersion(ctx, targetVersion)
		}
		dryRunValidate = func(ctx context.Context) (semversion.Number, error) {
			return targetVersion, c.upgraderService.CanUpgradeControllerToVersion(ctx, targetVersion)
		}
	case !hasTargetVersion && hasStreamChange:
		upgrader = func(ctx context.Context) error {
			version, err := c.upgraderService.UpgradeControllerWithStream(ctx, targetStream)
			targetVersion = version
			return err
		}
		dryRunValidate = func(ctx context.Context) (semversion.Number, error) {
			return c.upgraderService.CanUpgradeControllerWithStream(ctx, targetStream)
		}
	case !hasTargetVersion && !hasStreamChange:
		upgrader = func(ctx context.Context) error {
			version, err := c.upgraderService.UpgradeController(ctx)
			targetVersion = version
			return err
		}
		dryRunValidate = func(ctx context.Context) (semversion.Number, error) {
			return c.upgraderService.CanUpgradeController(ctx)
		}
	}

	// Return early and don't perform an actual upgrade if it's a dry run.
	if arg.DryRun {
		targetVersion, err := dryRunValidate(ctx)
		return c.mapResponse(err, targetVersion, arg)
	}

	// Invoke the upgrade here.
	err = upgrader(ctx)
	// Map the corresponding result and errors.
	return c.mapResponse(err, targetVersion, arg)
}

// UpgradeModel upgrades a controller and the model hosting the controller.
func (c *ControllerUpgraderAPI) UpgradeModel(ctx context.Context, arg params.UpgradeModelParams) (params.UpgradeModelResult, error) {
	var result params.UpgradeModelResult

	modelTag, err := names.ParseModelTag(arg.ModelTag)
	if err != nil {
		return result, errors.Capture(err)
	}
	allowed, err := c.canUpgrade(ctx, modelTag)
	if err != nil {
		return result, errors.Capture(err)
	}
	if !allowed {
		return result, errors.Capture(errors.New("unauthorized to upgrade model").Add(coreerrors.Unauthorized))
	}
	if err := c.check.ChangeAllowed(ctx); err != nil {
		return result, errors.Capture(err)
	}

	return c.doUpgrade(ctx, arg)
}
