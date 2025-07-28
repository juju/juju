// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelupgrader

import (
	"context"
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/names/v6"

	"github.com/juju/juju/apiserver/authentication"
	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/base"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/life"
	corelogger "github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/machine"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/domain/modelagent"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/docker"
	"github.com/juju/juju/internal/docker/registry"
	internalerrors "github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/upgrades/upgradevalidation"
	"github.com/juju/juju/rpc/params"
)

// ModelAgentService provides access to the Juju agent version for the model.
type ModelAgentService interface {
	// GetModelTargetAgentVersion returns the target agent version for the
	// entire model.
	GetModelTargetAgentVersion(context.Context) (semversion.Number, error)

	// UpgradeModelTargetAgentVersionTo upgrades a model to a new target agent
	// version. All agents that run on behalf of entities within the model will be
	// eventually upgraded to the new version after this call successfully returns.
	//
	// The version supplied must not be a downgrade from the current target agent
	// version of the model. It must also not be greater than the maximum supported
	// version of the controller.
	UpgradeModelTargetAgentVersionTo(context.Context, semversion.Number) error

	// UpgradeModelTargetAgentVersionStreamTo upgrades a model to a new target agent
	// version and updates the agent stream that is in use. All agents that run on
	// behalf of entities within the model will be eventually upgraded to the new
	// version after this call successfully returns.
	//
	// The version supplied must not be a downgrade from the current target agent
	// version of the model. It must also not be greater than the maximum supported
	// version of the controller.
	UpgradeModelTargetAgentVersionStreamTo(context.Context, semversion.Number, modelagent.AgentStream) error
}

// UpgradeService is an interface that allows us to check if the model
// is currently upgrading.
type UpgradeService interface {
	IsUpgrading(context.Context) (bool, error)
}

// MachineService provides access to machine base information.
type MachineService interface {
	// AllMachineNames returns the names of all machines in the model.
	AllMachineNames(ctx context.Context) ([]machine.Name, error)
	// GetMachineBase returns the base for the given machine.
	//
	// The following errors may be returned:
	// - [machineerrors.MachineNotFound] if the machine does not exist.
	GetMachineBase(ctx context.Context, mName machine.Name) (base.Base, error)
}

// ControllerConfigService is an interface that allows us to get the
// controller config.
type ControllerConfigService interface {
	ControllerConfig(context.Context) (controller.Config, error)
}

// ModelInfoService is the interface that provides access to the model info
// service.
type ModelInfoService interface {
	// GetModelInfo returns the readonly model information for the model in
	// question.
	GetModelInfo(context.Context) (coremodel.ModelInfo, error)
}

// ModelService is the interface that provides access to the model service.
type ModelService interface {
	// ListModelUUIDs returns a list of all model UUIDs in the controller that are
	// active.
	ListAllModels(context.Context) ([]coremodel.Model, error)
}

// ModelUpgraderAPI implements the model upgrader interface and is
// the concrete implementation of the api end point.
type ModelUpgraderAPI struct {
	controllerTag names.ControllerTag
	check         common.BlockCheckerInterface
	authorizer    facade.Authorizer
	toolsFinder   common.ToolsFinder

	modelAgentServiceGetter func(ctx context.Context, modelUUID coremodel.UUID) (ModelAgentService, error)
	machineServiceGetter    func(ctx context.Context, modelUUID coremodel.UUID) (MachineService, error)
	controllerAgentService  ModelAgentService
	controllerConfigService ControllerConfigService
	modelAgentService       ModelAgentService
	modelInfoService        ModelInfoService
	modelService            ModelService
	upgradeService          UpgradeService
	machineService          MachineService

	registryAPIFunc func(repoDetails docker.ImageRepoDetails) (registry.Registry, error)
	logger          corelogger.Logger
}

// NewModelUpgraderAPI creates a new api server endpoint for managing
// models.
func NewModelUpgraderAPI(
	controllerUUID string,
	toolsFinder common.ToolsFinder,
	blockChecker common.BlockCheckerInterface,
	authorizer facade.Authorizer,
	registryAPIFunc func(docker.ImageRepoDetails) (registry.Registry, error),
	modelAgentServiceGetter func(ctx context.Context, modelUUID coremodel.UUID) (ModelAgentService, error),
	machineServiceGetter func(ctx context.Context, modelUUID coremodel.UUID) (MachineService, error),
	controllerAgentService ModelAgentService,
	controllerConfigService ControllerConfigService,
	modelAgentService ModelAgentService,
	machineService MachineService,
	modelInfoService ModelInfoService,
	modelService ModelService,
	upgradeService UpgradeService,
	logger corelogger.Logger,
) (*ModelUpgraderAPI, error) {
	if !authorizer.AuthClient() {
		return nil, apiservererrors.ErrPerm
	}

	return &ModelUpgraderAPI{
		controllerTag:           names.NewControllerTag(controllerUUID),
		check:                   blockChecker,
		authorizer:              authorizer,
		toolsFinder:             toolsFinder,
		registryAPIFunc:         registryAPIFunc,
		upgradeService:          upgradeService,
		modelAgentServiceGetter: modelAgentServiceGetter,
		modelAgentService:       modelAgentService,
		machineServiceGetter:    machineServiceGetter,
		machineService:          machineService,
		modelInfoService:        modelInfoService,
		modelService:            modelService,
		controllerAgentService:  controllerAgentService,
		controllerConfigService: controllerConfigService,
		logger:                  logger,
	}, nil
}

func (m *ModelUpgraderAPI) canUpgrade(ctx context.Context, model names.ModelTag) error {
	err := m.authorizer.HasPermission(
		ctx,
		permission.SuperuserAccess,
		m.controllerTag,
	)
	if err != nil && !errors.Is(err, authentication.ErrorEntityMissingPermission) {
		return errors.Trace(err)
	}
	if err == nil {
		return nil
	}

	return m.authorizer.HasPermission(ctx, permission.WriteAccess, model)
}

// ConfigSource describes a type that is able to provide config.
// Abstracted primarily for testing.
type ConfigSource interface {
	Config() (*config.Config, error)
}

// AbortModelUpgrade returns not supported, as it's not possible to move
// back to a prior version.
func (m *ModelUpgraderAPI) AbortModelUpgrade(ctx context.Context, arg params.ModelParam) error {
	return errors.NotSupportedf("abort model upgrade")
}

// UpgradeModel upgrades a model.
func (m *ModelUpgraderAPI) UpgradeModel(ctx context.Context, arg params.UpgradeModelParams) (result params.UpgradeModelResult, err error) {
	m.logger.Tracef(ctx, "UpgradeModel arg %#v", arg)
	targetVersion := arg.TargetVersion
	defer func() {
		if err == nil {
			result.ChosenVersion = targetVersion
		}
	}()

	modelTag, err := names.ParseModelTag(arg.ModelTag)
	if err != nil {
		return result, errors.Trace(err)
	}
	if err := m.canUpgrade(ctx, modelTag); err != nil {
		return result, err
	}

	if err := m.check.ChangeAllowed(ctx); err != nil {
		return result, errors.Trace(err)
	}

	controllerCfg, err := m.controllerConfigService.ControllerConfig(ctx)
	if err != nil {
		return result, errors.Trace(err)
	}

	model, err := m.modelInfoService.GetModelInfo(ctx)
	if err != nil {
		return result, errors.Trace(err)
	}

	// TODO (tlm): Look at adding this check back in when upgrading logic lives
	// in a domain. More then likely this check is incorrect here as it should
	// be done at the stage where set the version upgrade into the database.
	// It might be true now but not in x seconds time.
	//if model.Life() != state.Alive {
	//	result.Error = apiservererrors.ServerError(errors.NewNotValid(nil, "model is not alive"))
	//	return result, nil
	//}

	currentVersion, err := m.modelAgentService.GetModelTargetAgentVersion(ctx)
	if err != nil {
		return result, errors.Trace(err)
	}

	// For non controller models, we use the exact controller
	// model version to upgrade to, unless an explicit target
	// has been specified.
	useControllerVersion := false
	if !model.IsControllerModel {
		vers, err := m.controllerAgentService.GetModelTargetAgentVersion(ctx)
		if err != nil {
			return result, errors.Trace(err)
		}
		if targetVersion == semversion.Zero || targetVersion.Compare(vers) == 0 {
			targetVersion = vers
			useControllerVersion = true
		} else if vers.Compare(targetVersion.ToPatch()) < 0 {
			return result, errors.Errorf("cannot upgrade to a version %q greater than that of the controller %q", targetVersion, vers)
		}
	}
	if !useControllerVersion {
		m.logger.Debugf(ctx, "deciding target version for model upgrade, from %q to %q for stream %q", currentVersion, targetVersion, arg.AgentStream)
		args := common.FindAgentsParams{
			AgentStream:   arg.AgentStream,
			ControllerCfg: controllerCfg,
			ModelType:     model.Type,
		}
		if targetVersion == semversion.Zero {
			args.MajorVersion = currentVersion.Major
			args.MinorVersion = currentVersion.Minor
		} else {
			args.Number = targetVersion
		}
		targetVersion, err = m.decideVersion(ctx, currentVersion, args)
		if errors.Is(errors.Cause(err), errors.NotFound) || errors.Is(errors.Cause(err), errors.AlreadyExists) {
			result.Error = apiservererrors.ServerError(err)
			return result, nil
		}

		if err != nil {
			return result, errors.Trace(err)
		}
	}

	if err := m.validateModelUpgrade(ctx, false, modelTag, targetVersion, model); err != nil {
		result.Error = apiservererrors.ServerError(err)
		return result, nil
	}
	if arg.DryRun {
		return result, nil
	}

	if arg.AgentStream != "" {
		agentStream, err := encodeAgentStream(arg.AgentStream)
		if err != nil {
			return result, errors.Trace(err)
		}
		if err := m.modelAgentService.UpgradeModelTargetAgentVersionStreamTo(ctx, targetVersion, agentStream); err != nil {
			return result, errors.Trace(err)
		}
	} else {
		if err := m.modelAgentService.UpgradeModelTargetAgentVersionTo(ctx, targetVersion); err != nil {
			return result, errors.Trace(err)
		}
	}
	return result, nil
}

func encodeAgentStream(agentStream string) (modelagent.AgentStream, error) {
	switch agentStream {
	case "released":
		return modelagent.AgentStreamReleased, nil
	case "proposed":
		return modelagent.AgentStreamProposed, nil
	case "testing":
		return modelagent.AgentStreamTesting, nil
	case "devel":
		return modelagent.AgentStreamDevel, nil
	default:
		return modelagent.AgentStream(-1), internalerrors.Errorf(
			"agent stream %q is not recognised as a valid value", agentStream,
		).Add(coreerrors.NotValid)
	}
}

func (m *ModelUpgraderAPI) validateModelUpgrade(
	ctx context.Context,
	force bool, modelTag names.ModelTag, targetVersion semversion.Number,
	model coremodel.ModelInfo,
) (err error) {
	var blockers *upgradevalidation.ModelUpgradeBlockers
	defer func() {
		if err == nil && blockers != nil {
			err = errors.NewNotSupported(nil,
				fmt.Sprintf(
					"cannot upgrade to %q due to issues with these models:\n%s",
					targetVersion, blockers,
				),
			)
		}
	}()

	validationServices := upgradevalidation.ValidatorServices{
		ModelAgentService: m.modelAgentService,
		MachineService:    m.machineService,
	}

	if !model.IsControllerModel {
		validators := upgradevalidation.ValidatorsForModelUpgrade(force, targetVersion)
		checker := upgradevalidation.NewModelUpgradeCheck(model.UUID.String(), validationServices, validators...)
		blockers, err = checker.Validate(ctx)
		if err != nil {
			return errors.Trace(err)
		}
		return
	}

	checker := upgradevalidation.NewModelUpgradeCheck(
		model.UUID.String(), validationServices,
		upgradevalidation.ValidatorsForControllerModelUpgrade(targetVersion)...,
	)
	blockers, err = checker.Validate(ctx)
	if err != nil {
		return errors.Trace(err)
	}

	models, err := m.modelService.ListAllModels(ctx)
	if err != nil {
		return errors.Trace(err)
	}

	for _, model := range models {
		if model.UUID.String() == modelTag.Id() {
			// We have done checks for controller model above already.
			continue
		}

		if model.Life != life.Alive {
			m.logger.Tracef(ctx, "skipping upgrade check for dying/dead model %s", model.Name)
			continue
		}

		validators := upgradevalidation.ModelValidatorsForControllerModelUpgrade(targetVersion)

		modelNameKey := fmt.Sprintf("%s/%s", model.Qualifier, model.Name)
		modelAgentService, err := m.modelAgentServiceGetter(ctx, model.UUID)
		if err != nil {
			return errors.Trace(err)
		}
		machineService, err := m.machineServiceGetter(ctx, model.UUID)
		if err != nil {
			return errors.Trace(err)
		}
		validationServices := upgradevalidation.ValidatorServices{
			ModelAgentService: modelAgentService,
			MachineService:    machineService,
		}
		checker := upgradevalidation.NewModelUpgradeCheck(modelNameKey, validationServices, validators...)
		blockersForModel, err := checker.Validate(ctx)
		if err != nil {
			return errors.Annotatef(err, "validating model %q for controller upgrade", model.Name)
		}
		if blockersForModel == nil {
			// all good.
			continue
		}
		if blockers == nil {
			blockers = blockersForModel
			continue
		}
		blockers.Join(blockersForModel)
	}
	return
}
