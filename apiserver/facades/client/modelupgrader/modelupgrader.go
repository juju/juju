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
	"github.com/juju/juju/domain/modelagent"
	modelagenterrors "github.com/juju/juju/domain/modelagent/errors"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/rpc/params"
)

type ModelAgentService interface {
	UpgradeModelTargetAgentVersion(
		ctx context.Context,
	) (semversion.Number, error)
	UpgradeModelTargetAgentVersionStream(
		ctx context.Context,
		agentStream modelagent.AgentStream,
	) (semversion.Number, error)
	UpgradeModelTargetAgentVersionTo(
		ctx context.Context,
		desiredTargetVersion semversion.Number,
	) error
	UpgradeModelTargetAgentVersionStreamTo(
		ctx context.Context,
		desiredTargetVersion semversion.Number,
		agentStream modelagent.AgentStream,
	) error
	RunPreUpgradeChecks(
		ctx context.Context,
	) (semversion.Number, error)
	RunPreUpgradeChecksToVersion(
		ctx context.Context,
		desiredTargetVersion semversion.Number,
	) (semversion.Number, error)
	RunPreUpgradeChecksWithStream(
		ctx context.Context,
		stream modelagent.AgentStream,
	) (semversion.Number, error)
	RunPreUpgradeChecksToVersionWithStream(
		ctx context.Context,
		desiredTargetVersion semversion.Number,
		stream modelagent.AgentStream,
	) (semversion.Number, error)
}

type ModelUpgraderAPI struct {
	authorizer facade.Authorizer
	check      common.BlockCheckerInterface

	modelAgentService ModelAgentService

	controllerTag names.Tag
	modelTag      names.Tag
}

func NewModelUpgraderAPI(
	controllerTag names.Tag,
	modelTag names.Tag,
	authorizer facade.Authorizer,
	check common.BlockCheckerInterface,
	modelAgentService ModelAgentService,
) (*ModelUpgraderAPI, error) {
	if !authorizer.AuthClient() {
		return nil, apiservererrors.ErrPerm
	}

	return &ModelUpgraderAPI{
		authorizer:        authorizer,
		check:             check,
		modelAgentService: modelAgentService,
		controllerTag:     controllerTag,
		modelTag:          modelTag,
	}, nil
}

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
	if err != nil && !errors.Is(err, authentication.ErrorEntityMissingPermission) {
		return false, errors.Capture(err)
	}
	if err == nil {
		return true, nil
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

func (m *ModelUpgraderAPI) AbortModelUpgrade(
	_ context.Context,
	_ params.ModelParam,
) error {
	return errors.New("aborting model upgrades is not supported").
		Add(coreerrors.NotSupported)
}

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

func (m *ModelUpgraderAPI) dryRunUpgrade(
	ctx context.Context,
	arg params.UpgradeModelParams,
) (params.UpgradeModelResult, error) {
	var (
		hasStreamChange      = arg.AgentStream != ""
		hasTargetVersion     = arg.TargetVersion != semversion.Zero
		targetStream         modelagent.AgentStream
		desiredTargetVersion = arg.TargetVersion
		dryRunValidate       func(context.Context) (semversion.Number, error)
		result               params.UpgradeModelResult
		err                  error
	)

	// Parse the agent stream.
	if arg.AgentStream != "" {
		targetStream, err = modelagent.AgentStreamFromCoreAgentStream(
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

func (m *ModelUpgraderAPI) runUpgrade(
	ctx context.Context,
	arg params.UpgradeModelParams,
) (params.UpgradeModelResult, error) {
	var (
		hasStreamChange  = arg.AgentStream != ""
		hasTargetVersion = arg.TargetVersion != semversion.Zero
		targetStream     modelagent.AgentStream
		targetVersion    = arg.TargetVersion
		upgrader         func(context.Context) error
		result           params.UpgradeModelResult
		err              error
	)

	// Parse the agent stream.
	if arg.AgentStream != "" {
		targetStream, err = modelagent.AgentStreamFromCoreAgentStream(
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
			return m.modelAgentService.UpgradeModelTargetAgentVersionTo(
				ctx,
				targetVersion,
			)
		}
	case !hasTargetVersion && hasStreamChange:
		upgrader = func(ctx context.Context) error {
			version, err := m.modelAgentService.
				UpgradeModelTargetAgentVersionStream(
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
