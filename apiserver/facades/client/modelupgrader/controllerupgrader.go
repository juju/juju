package modelupgrader

import (
	"context"
	"fmt"
	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/core/agentbinary"
	coreerrors "github.com/juju/juju/core/errors"
	corelogger "github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/semversion"
	controllerupgradererrors "github.com/juju/juju/domain/controllerupgrader/errors"
	"github.com/juju/juju/domain/modelagent"
	modelagenterrors "github.com/juju/juju/domain/modelagent/errors"
	internalerrors "github.com/juju/juju/internal/errors"
	"github.com/juju/names/v6"

	"github.com/juju/errors"
	"github.com/juju/juju/rpc/params"
)

// ControllerUpgraderService mirrors the controller upgrader service
// so we can easily mock it for unit tests.
type ControllerUpgraderService interface {
	UpgradeController(ctx context.Context) (semversion.Number, error)
	UpgradeControllerWithStream(
		ctx context.Context,
		stream modelagent.AgentStream,
	) (semversion.Number, error)
	UpgradeControllerToVersion(
		ctx context.Context,
		desiredVersion semversion.Number,
	) error
	UpgradeControllerToVersionAndStream(
		ctx context.Context,
		desiredVersion semversion.Number,
		stream modelagent.AgentStream,
	) error
}

// ControllerUpgraderAPI upgrades a controller and a model hosting the controller.
type ControllerUpgraderAPI struct {
	authorizer facade.Authorizer
	check      common.BlockCheckerInterface

	upgraderService ControllerUpgraderService

	logger corelogger.Logger
}

// NewControllerUpgraderAPI instantiates a new [ControllerUpgraderAPI].
func NewControllerUpgraderAPI(
	authorizer facade.Authorizer,
	check common.BlockCheckerInterface,
	upgraderService ControllerUpgraderService,
	logger corelogger.Logger,
) *ControllerUpgraderAPI {
	return &ControllerUpgraderAPI{
		authorizer:      authorizer,
		check:           check,
		upgraderService: upgraderService,
		logger:          logger,
	}
}

// AbortModelUpgrade returns not supported, as it's not possible to move
// back to a prior version.
func (c *ControllerUpgraderAPI) AbortModelUpgrade(ctx context.Context, arg params.ModelParam) error {
	return errors.NotSupportedf("abort model upgrade")
}

func (c *ControllerUpgraderAPI) canUpgrade(ctx context.Context, model names.ModelTag) error {
	return nil
}

// UpgradeModel upgrades a controller and the model hosting the controller.
// TODO(adisazhar123): support arg.dryRun.
func (c *ControllerUpgraderAPI) UpgradeModel(ctx context.Context, arg params.UpgradeModelParams) (params.UpgradeModelResult, error) {
	c.logger.Tracef(ctx, "UpgradeModel arg %#v", arg)
	var result params.UpgradeModelResult

	modelTag, err := names.ParseModelTag(arg.ModelTag)
	if err != nil {
		return result, errors.Trace(err)
	}
	if err := c.canUpgrade(ctx, modelTag); err != nil {
		return result, err
	}
	if err := c.check.ChangeAllowed(ctx); err != nil {
		return result, errors.Trace(err)
	}

	return c.doUpgrade(ctx, arg)
}

// doUpgrade has the responsibility of delegating the upgrade to the service. It determines which func to invoke
// by interrogating the values set in [params.UpgradeModelParams].
// A post-processing step is performed to map the errors returned from the service to ones the existing API
// conforms to.
func (c *ControllerUpgraderAPI) doUpgrade(ctx context.Context, arg params.UpgradeModelParams) (params.UpgradeModelResult, error) {
	var result params.UpgradeModelResult

	var targetVersion = arg.TargetVersion
	var err error

	// Delegate it to the service depending on what arguments
	// are supplied.
	if arg.TargetVersion == semversion.Zero && arg.AgentStream == "" {
		targetVersion, err = c.upgraderService.UpgradeController(ctx)
	} else if arg.TargetVersion == semversion.Zero && arg.AgentStream != "" {
		var stream modelagent.AgentStream
		stream, err = modelagent.AgentStreamFromCoreAgentStream(agentbinary.AgentStream(arg.AgentStream))
		if err == nil {
			targetVersion, err = c.upgraderService.UpgradeControllerWithStream(ctx, stream)
		}
	} else if arg.TargetVersion != semversion.Zero && arg.AgentStream == "" {
		err = c.upgraderService.UpgradeControllerToVersion(ctx, arg.TargetVersion)
	} else if arg.TargetVersion != semversion.Zero && arg.AgentStream != "" {
		var stream modelagent.AgentStream
		stream, err = modelagent.AgentStreamFromCoreAgentStream(agentbinary.AgentStream(arg.AgentStream))
		if err == nil {
			err = c.upgraderService.UpgradeControllerToVersionAndStream(ctx, arg.TargetVersion, stream)
		}
	}
	result.ChosenVersion = targetVersion

	// Map the errors to respect what the existing API returns.
	// We mirror as closely as possible to [UpgradeModel] func in [modelupgrader.ModelUpgraderAPI].
	switch {
	case errors.Is(err, controllerupgradererrors.MissingControllerBinaries):
		result.Error = apiservererrors.ServerError(err)
		return result, nil
	case errors.HasType[controllerupgradererrors.ControllerUpgradeBlocker](err):
		e := errors.NewNotSupported(nil,
			fmt.Sprintf(
				"cannot upgrade due to: %s", err.Error(),
			),
		)
		result.Error = apiservererrors.ServerError(e)
		return result, nil
	case errors.Is(err, controllerupgradererrors.VersionNotSupported):
		return result, errors.Errorf("cannot upgrade to a version %q greather than that of the controller", targetVersion)
	case errors.Is(err, modelagenterrors.AgentStreamNotValid):
		e := internalerrors.Errorf(
			"agent stream %q is not recognised as a valid value", arg.AgentStream,
		).Add(coreerrors.NotValid)
		return result, errors.Trace(e)
	case err != nil:
		return result, errors.Trace(err)
	}

	return result, nil
}
