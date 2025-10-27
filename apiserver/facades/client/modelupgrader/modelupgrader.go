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
	domainagentbinary "github.com/juju/juju/domain/agentbinary"
	modelagenterrors "github.com/juju/juju/domain/modelagent/errors"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/rpc/params"
)

// ModelAgentService mirrors the model agent service
// so we can easily mock it for unit tests.
// Func docs are cherry-picked from
//
//	[github.com/juju/juju/domain/modelagent/service.Service]. See there for more
//
// implementation details.
type ModelAgentService interface {
	// UpgradeModelTargetAgentVersion is responsible for upgrading the target
	// agent version of the current model to latest version available.
	// The version that is upgraded to is returned.
	UpgradeModelTargetAgentVersion(
		ctx context.Context,
	) (semversion.Number, error)

	// UpgradeModelTargetAgentVersionWithStream is responsible for upgrading the target
	// agent version of the current model to the latest version available. While
	// performing the upgrade the agent stream for the model will also be changed.
	// The version that is upgraded to is returned.
	UpgradeModelTargetAgentVersionWithStream(
		ctx context.Context,
		stream domainagentbinary.Stream,
	) (semversion.Number, error)

	// UpgradeModelAgentToTargetVersion upgrades a model to a new target agent
	// version. All agents that run on behalf of entities within the model will be
	// eventually upgraded to the new version after this call successfully returns.
	UpgradeModelAgentToTargetVersion(
		ctx context.Context,
		desiredTargetVersion semversion.Number,
	) error

	// UpgradeModelTargetAgentVersionStreamTo upgrades a model to a new target agent
	// version and updates the agent stream that is in use. All agents that run on
	// behalf of entities within the model will be eventually upgraded to the new
	// version after this call successfully returns.
	UpgradeModelTargetAgentVersionStreamTo(
		ctx context.Context,
		desiredTargetVersion semversion.Number,
		stream domainagentbinary.Stream,
	) error

	// RunPreUpgradeChecks determines whether the model can be upgraded
	// to the latest available patch version. It returns the recommended version
	// to upgrade to.
	RunPreUpgradeChecks(
		ctx context.Context,
	) (semversion.Number, error)

	// RunPreUpgradeChecksToVersion determines whether the controller can be
	// safely upgraded to the specified version. It performs validation checks
	// to ensure that the target version is valid and that the upgrade
	// can proceed.
	// It returns the currently running version.
	RunPreUpgradeChecksToVersion(
		ctx context.Context,
		desiredTargetVersion semversion.Number,
	) (semversion.Number, error)

	// RunPreUpgradeChecksWithStream determines whether the model can be
	// upgraded to the latest available patch version within the specified agent
	// stream. It returns the desired version that the model can upgrade to
	// if all validation checks pass.
	// It returns the recommended version to upgrade to.
	RunPreUpgradeChecksWithStream(
		ctx context.Context,
		stream domainagentbinary.Stream,
	) (semversion.Number, error)

	// RunPreUpgradeChecksToVersionWithStream determines whether the model
	// can be safely upgraded to the specified version within the given
	// agent stream.
	// It returns the currently running version.
	RunPreUpgradeChecksToVersionWithStream(
		ctx context.Context,
		desiredTargetVersion semversion.Number,
		stream domainagentbinary.Stream,
	) (semversion.Number, error)
}

// ModelUpgraderAPI upgrades the model.
type ModelUpgraderAPI struct {
	authorizer facade.Authorizer
	check      common.BlockCheckerInterface

	modelAgentService ModelAgentService

	controllerTag names.Tag
	modelTag      names.Tag
}

// NewModelUpgraderAPI instantiates a new [ModelUpgraderAPI].
func NewModelUpgraderAPI(
	controllerTag names.Tag,
	modelTag names.Tag,
	authorizer facade.Authorizer,
	check common.BlockCheckerInterface,
	modelAgentService ModelAgentService,
) *ModelUpgraderAPI {
	return &ModelUpgraderAPI{
		authorizer:        authorizer,
		check:             check,
		modelAgentService: modelAgentService,
		controllerTag:     controllerTag,
		modelTag:          modelTag,
	}
}

// canUpgrade has the responsibility to determine whether there is sufficient
// permission to perform an upgrade.
func (m *ModelUpgraderAPI) canUpgrade(
	ctx context.Context,
	model names.ModelTag,
) (bool, error) {
	if model.Id() != m.modelTag.Id() {
		return false, nil
	}
	err := m.authorizer.HasPermission(
		ctx,
		permission.SuperuserAccess,
		m.controllerTag,
	)
	if err == nil {
		return true, nil
	} else if !errors.Is(err, authentication.ErrorEntityMissingPermission) {
		return false, errors.Capture(err)
	}

	err = m.authorizer.HasPermission(
		ctx,
		permission.WriteAccess,
		model,
	)
	if errors.Is(err, authentication.ErrorEntityMissingPermission) {
		return false, nil
	} else if err != nil {
		return false, errors.Capture(err)
	}
	return true, nil
}

// mapError takes in a supplied error from the upgrade service and maps the
// corresponding error to:
//   - a [params.Error] in which the call site has to assign to the [Error] field
//     in [params.UpgradeModelResult] object, OR
//   - a [error] return value
func (m *ModelUpgraderAPI) mapError(
	inErr error,
	targetVersion semversion.Number,
	arg params.UpgradeModelParams,
) (*params.Error, error) {
	var (
		paramsErr *params.Error
		outErr    error
	)

	switch {
	case errors.Is(inErr, modelagenterrors.MissingAgentBinaries):
		paramsErr = apiservererrors.ParamsErrorf(
			params.CodeNotFound,
			"model agent binaries are not available for version %q",
			targetVersion,
		)
	case errors.HasType[modelagenterrors.ModelUpgradeBlocker](inErr):
		blockedErr, _ := errors.AsType[modelagenterrors.ModelUpgradeBlocker](inErr)
		paramsErr = apiservererrors.ParamsErrorf(
			params.CodeNotSupported,
			"model upgrading is blocked for reason: %s",
			blockedErr.Reason,
		)
	case errors.Is(inErr, modelagenterrors.DowngradeNotSupported):
		paramsErr = apiservererrors.ParamsErrorf(
			params.CodeNotSupported,
			"cannot upgrade the model agent to version %q because it is "+
				"lower than the current running version",
			targetVersion,
		)
	case errors.Is(inErr, modelagenterrors.AgentVersionNotSupported):
		outErr = errors.New(
			"cannot upgrade the model to a version that is more than " +
				"a patch version increase",
		).Add(coreerrors.NotValid)
	case errors.Is(inErr, coreerrors.NotValid):
		outErr = errors.Errorf(
			"agent stream %q is not a recognised valid value",
			arg.AgentStream,
		).Add(coreerrors.NotValid)
	case inErr != nil:
		return nil, apiservererrors.ServerError(inErr)
	}

	return paramsErr, outErr
}

// AbortModelUpgrade returns not supported, as it's not possible to move
// back to a prior version.
func (m *ModelUpgraderAPI) AbortModelUpgrade(
	_ context.Context,
	_ params.ModelParam,
) error {
	return errors.New("aborting model upgrades is not supported").
		Add(coreerrors.NotSupported)
}

// UpgradeModel upgrades the target agent version of the current model.
// It first validates that the caller has sufficient permissions and that
// the model is allowed to change. Depending on the provided parameters,
// it either performs a dry-run validation or executes the actual upgrade.
func (m *ModelUpgraderAPI) UpgradeModel(
	ctx context.Context,
	arg params.UpgradeModelParams,
) (params.UpgradeModelResult, error) {
	var result params.UpgradeModelResult

	modelTag, err := names.ParseModelTag(arg.ModelTag)
	if err != nil {
		return result, errors.Capture(err)
	}
	allowed, err := m.canUpgrade(ctx, modelTag)
	if err != nil {
		return result, errors.Capture(err)
	}
	if !allowed {
		return result,
			errors.Capture(errors.New("unauthorized to upgrade model").
				Add(coreerrors.Unauthorized))
	}
	if err := m.check.ChangeAllowed(ctx); err != nil {
		return result, errors.Capture(err)
	}

	if arg.DryRun {
		return m.dryRunUpgrade(ctx, arg)
	}
	return m.runUpgrade(ctx, arg)
}

// dryRunUpgrade has the responsibility of delegating the validation to the
// service before an upgrade is performed. We don't perform the real upgrade
// here but rather validation checks to ensure we are in a good condition before
// should a real upgrade occur.
func (m *ModelUpgraderAPI) dryRunUpgrade(
	ctx context.Context,
	arg params.UpgradeModelParams,
) (params.UpgradeModelResult, error) {
	var (
		hasStreamChange      = arg.AgentStream != ""
		hasTargetVersion     = arg.TargetVersion != semversion.Zero
		targetStream         domainagentbinary.Stream
		desiredTargetVersion = arg.TargetVersion
		dryRunValidate       func(context.Context) (semversion.Number, error)
		result               params.UpgradeModelResult
		err                  error
	)

	// Parse the agent stream.
	if arg.AgentStream != "" {
		targetStream, err = domainagentbinary.StreamFromCoreAgentBinaryStream(
			agentbinary.AgentStream(arg.AgentStream),
		)
		if err != nil {
			if errors.Is(err, coreerrors.NotValid) {
				return result, errors.Capture(errors.Errorf(
					"agent stream %q is not recognised as a valid value",
					arg.AgentStream,
				).Add(coreerrors.NotValid))
			}
			return result, errors.Capture(err)
		}
	}

	// Delegate it to the service depending on what arguments are supplied.
	switch {
	case hasTargetVersion && hasStreamChange:
		dryRunValidate = func(ctx context.Context) (semversion.Number, error) {
			_, err := m.modelAgentService.
				RunPreUpgradeChecksToVersionWithStream(
					ctx,
					desiredTargetVersion,
					targetStream,
				)
			return desiredTargetVersion, err
		}
	case hasTargetVersion && !hasStreamChange:
		dryRunValidate = func(ctx context.Context) (semversion.Number, error) {
			_, err := m.modelAgentService.
				RunPreUpgradeChecksToVersion(
					ctx,
					desiredTargetVersion,
				)
			return desiredTargetVersion, err
		}
	case !hasTargetVersion && hasStreamChange:
		dryRunValidate = func(ctx context.Context) (semversion.Number, error) {
			return m.modelAgentService.RunPreUpgradeChecksWithStream(
				ctx,
				targetStream,
			)
		}
	default:
		dryRunValidate = func(ctx context.Context) (semversion.Number, error) {
			return m.modelAgentService.RunPreUpgradeChecks(ctx)
		}
	}

	desiredTargetVersion, err = dryRunValidate(ctx)
	paramErr, err := m.mapError(err, desiredTargetVersion, arg)
	result.ChosenVersion = desiredTargetVersion
	result.Error = paramErr
	return result, errors.Capture(err)
}

// runUpgrade has the responsibility of delegating the upgrade to the service.
// It determines which func to invoke by interrogating the values set in
// [params.UpgradeModelParams]. A post-processing step is performed to map the
// errors returned from the service to ones the existing API conforms to.
func (m *ModelUpgraderAPI) runUpgrade(
	ctx context.Context,
	arg params.UpgradeModelParams,
) (params.UpgradeModelResult, error) {
	var (
		hasStreamChange  = arg.AgentStream != ""
		hasTargetVersion = arg.TargetVersion != semversion.Zero
		targetStream     domainagentbinary.Stream
		targetVersion    = arg.TargetVersion
		upgrader         func(context.Context) error
		result           params.UpgradeModelResult
		err              error
	)

	// Parse the agent stream.
	if arg.AgentStream != "" {
		targetStream, err = domainagentbinary.StreamFromCoreAgentBinaryStream(
			agentbinary.AgentStream(arg.AgentStream),
		)
		if err != nil {
			if errors.Is(err, coreerrors.NotValid) {
				return result, errors.Capture(errors.Errorf(
					"agent stream %q is not recognised as a valid value",
					arg.AgentStream,
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
			return m.modelAgentService.UpgradeModelTargetAgentVersionStreamTo(
				ctx,
				targetVersion,
				targetStream,
			)
		}
	case hasTargetVersion && !hasStreamChange:
		upgrader = func(ctx context.Context) error {
			return m.modelAgentService.UpgradeModelAgentToTargetVersion(
				ctx,
				targetVersion,
			)
		}
	case !hasTargetVersion && hasStreamChange:
		upgrader = func(ctx context.Context) error {
			version, err := m.modelAgentService.
				UpgradeModelTargetAgentVersionWithStream(
					ctx,
					targetStream,
				)
			targetVersion = version
			return err
		}
	default:
		upgrader = func(ctx context.Context) error {
			version, err := m.modelAgentService.UpgradeModelTargetAgentVersion(ctx)
			targetVersion = version
			return err
		}
	}

	// Invoke the upgrade here.
	err = upgrader(ctx)
	paramErr, err := m.mapError(err, targetVersion, arg)
	result.ChosenVersion = targetVersion
	result.Error = paramErr
	return result, errors.Capture(err)
}
