// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelupgrader

import (
	"context"
	stdcontext "context"
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"github.com/juju/version/v2"

	"github.com/juju/juju/apiserver/authentication"
	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/controller"
	corelogger "github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/environs"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/envcontext"
	"github.com/juju/juju/internal/docker"
	"github.com/juju/juju/internal/docker/registry"
	"github.com/juju/juju/internal/upgrades/upgradevalidation"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

// UpgradeService is an interface that allows us to check if the model
// is currently upgrading.
type UpgradeService interface {
	IsUpgrading(context.Context) (bool, error)
}

// ControllerConfigService is an interface that allows us to get the
// controller config.
type ControllerConfigService interface {
	ControllerConfig(context.Context) (controller.Config, error)
}

// ModelUpgraderAPI implements the model upgrader interface and is
// the concrete implementation of the api end point.
type ModelUpgraderAPI struct {
	controllerTag               names.ControllerTag
	statePool                   StatePool
	check                       common.BlockCheckerInterface
	authorizer                  facade.Authorizer
	toolsFinder                 common.ToolsFinder
	apiUser                     names.UserTag
	credentialInvalidatorGetter envcontext.ModelCredentialInvalidatorGetter
	newEnviron                  common.NewEnvironFunc
	controllerConfigService     ControllerConfigService
	upgradeService              UpgradeService

	registryAPIFunc         func(repoDetails docker.ImageRepoDetails) (registry.Registry, error)
	environscloudspecGetter func(context.Context, names.ModelTag) (environscloudspec.CloudSpec, error)
	logger                  corelogger.Logger
}

// NewModelUpgraderAPI creates a new api server endpoint for managing
// models.
func NewModelUpgraderAPI(
	controllerTag names.ControllerTag,
	stPool StatePool,
	toolsFinder common.ToolsFinder,
	newEnviron common.NewEnvironFunc,
	blockChecker common.BlockCheckerInterface,
	authorizer facade.Authorizer,
	credentialInvalidatorGetter envcontext.ModelCredentialInvalidatorGetter,
	registryAPIFunc func(docker.ImageRepoDetails) (registry.Registry, error),
	environscloudspecGetter func(context.Context, names.ModelTag) (environscloudspec.CloudSpec, error),
	controllerConfigService ControllerConfigService,
	upgradeService UpgradeService,
	logger corelogger.Logger,
) (*ModelUpgraderAPI, error) {
	if !authorizer.AuthClient() {
		return nil, apiservererrors.ErrPerm
	}
	// Since we know this is a user tag (because AuthClient is true),
	// we just do the type assertion to the UserTag.
	apiUser, _ := authorizer.GetAuthTag().(names.UserTag)

	return &ModelUpgraderAPI{
		controllerTag:               controllerTag,
		statePool:                   stPool,
		check:                       blockChecker,
		authorizer:                  authorizer,
		toolsFinder:                 toolsFinder,
		apiUser:                     apiUser,
		credentialInvalidatorGetter: credentialInvalidatorGetter,
		newEnviron:                  newEnviron,
		registryAPIFunc:             registryAPIFunc,
		environscloudspecGetter:     environscloudspecGetter,
		upgradeService:              upgradeService,
		controllerConfigService:     controllerConfigService,
		logger:                      logger,
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
func (m *ModelUpgraderAPI) AbortModelUpgrade(ctx stdcontext.Context, arg params.ModelParam) error {
	return errors.NotSupportedf("abort model upgrade")
}

// UpgradeModel upgrades a model.
func (m *ModelUpgraderAPI) UpgradeModel(ctx stdcontext.Context, arg params.UpgradeModelParams) (result params.UpgradeModelResult, err error) {
	m.logger.Tracef("UpgradeModel arg %#v", arg)
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

	// We now need to access the state pool for that given model.
	st, err := m.statePool.Get(modelTag.Id())
	if err != nil {
		return result, errors.Trace(err)
	}
	defer st.Release()

	controllerCfg, err := m.controllerConfigService.ControllerConfig(ctx)
	if err != nil {
		return result, errors.Trace(err)
	}
	model, err := st.Model()
	if err != nil {
		return result, errors.Trace(err)
	}

	if model.Life() != state.Alive {
		result.Error = apiservererrors.ServerError(errors.NewNotValid(nil, "model is not alive"))
		return result, nil
	}

	currentVersion, err := model.AgentVersion()
	if err != nil {
		return result, errors.Trace(err)
	}

	// For non controller models, we use the exact controller
	// model version to upgrade to, unless an explicit target
	// has been specified.
	useControllerVersion := false
	if !model.IsControllerModel() {
		ctrlModel, err := m.statePool.ControllerModel()
		if err != nil {
			return result, errors.Trace(err)
		}
		vers, err := ctrlModel.AgentVersion()
		if err != nil {
			return result, errors.Trace(err)
		}
		if targetVersion == version.Zero || targetVersion.Compare(vers) == 0 {
			targetVersion = vers
			useControllerVersion = true
		} else if vers.Compare(targetVersion.ToPatch()) < 0 {
			return result, errors.Errorf("cannot upgrade to a version %q greater than that of the controller %q", targetVersion, vers)
		}
	}
	if !useControllerVersion {
		m.logger.Debugf("deciding target version for model upgrade, from %q to %q for stream %q", currentVersion, targetVersion, arg.AgentStream)
		args := common.FindAgentsParams{
			AgentStream:   arg.AgentStream,
			ControllerCfg: controllerCfg,
			ModelType:     model.Type(),
		}
		if targetVersion == version.Zero {
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

	// Before changing the agent version to trigger an upgrade or downgrade,
	// we'll do a very basic check to ensure the environment is accessible.
	envOrBroker, err := m.newEnviron(ctx)
	if err != nil {
		return result, errors.Trace(err)
	}
	invalidatorFunc, err := m.credentialInvalidatorGetter()
	if err != nil {
		return result, errors.Trace(err)
	}
	callCtx := envcontext.WithCredentialInvalidator(ctx, invalidatorFunc)
	if err := preCheckEnvironForUpgradeModel(
		callCtx, envOrBroker, model.IsControllerModel(), currentVersion, targetVersion, m.logger,
	); err != nil {
		return result, errors.Trace(err)
	}

	if err := m.validateModelUpgrade(ctx, false, modelTag, targetVersion, st, model); err != nil {
		result.Error = apiservererrors.ServerError(err)
		return result, nil
	}
	if arg.DryRun {
		return result, nil
	}

	var agentStream *string
	if arg.AgentStream != "" {
		agentStream = &arg.AgentStream
	}
	if err := st.SetModelAgentVersion(targetVersion, agentStream, arg.IgnoreAgentVersions, shimUpgrader{
		upgradeService: m.upgradeService,
		ctx:            ctx,
	}); err != nil {
		return result, errors.Trace(err)
	}
	return result, nil
}

func preCheckEnvironForUpgradeModel(
	ctx envcontext.ProviderCallContext, env environs.BootstrapEnviron,
	controllerModel bool, currentVersion, targetVersion version.Number,
	logger corelogger.Logger,
) error {
	if err := environs.CheckProviderAPI(env, ctx); err != nil {
		return errors.Trace(err)
	}

	if !controllerModel {
		return nil
	}

	precheckEnv, ok := env.(environs.JujuUpgradePrechecker)
	if !ok {
		return nil
	}

	// skipTarget returns true if the from version is less than the target version
	// AND the target version is greater than the to version.
	// Borrowed from upgrades.opsIterator.
	skipTarget := func(from, target, to version.Number) bool {
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

	if err := precheckEnv.PreparePrechecker(); err != nil {
		return err
	}

	for _, op := range precheckEnv.PrecheckUpgradeOperations() {
		if skipTarget(currentVersion, op.TargetVersion, targetVersion) {
			logger.Debugf("ignoring precheck upgrade operation for version %s", op.TargetVersion)
			continue
		}
		logger.Debugf("running precheck upgrade operation for version %s", op.TargetVersion)
		for _, step := range op.Steps {
			logger.Debugf("running precheck step %q", step.Description())
			if err := step.Run(); err != nil {
				return errors.Annotatef(err, "Unable to upgrade to %s:", targetVersion)
			}
		}
	}
	return nil
}

func (m *ModelUpgraderAPI) validateModelUpgrade(
	ctx context.Context,
	force bool, modelTag names.ModelTag, targetVersion version.Number,
	st State, model Model,
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

	cloudspec, err := m.environscloudspecGetter(ctx, modelTag)
	if err != nil {
		return errors.Trace(err)
	}

	isControllerModel := model.IsControllerModel()
	if !isControllerModel {
		validators := upgradevalidation.ValidatorsForModelUpgrade(force, targetVersion, cloudspec)
		checker := upgradevalidation.NewModelUpgradeCheck(modelTag.Id(), m.statePool, st, model, validators...)
		blockers, err = checker.Validate()
		if err != nil {
			return errors.Trace(err)
		}
		return
	}

	checker := upgradevalidation.NewModelUpgradeCheck(
		modelTag.Id(), m.statePool, st, model,
		upgradevalidation.ValidatorsForControllerModelUpgrade(targetVersion, cloudspec)...,
	)
	blockers, err = checker.Validate()
	if err != nil {
		return errors.Trace(err)
	}

	modelUUIDs, err := st.AllModelUUIDs()
	if err != nil {
		return errors.Trace(err)
	}
	for _, modelUUID := range modelUUIDs {
		if modelUUID == modelTag.Id() {
			// We have done checks for controller model above already.
			continue
		}

		st, err := m.statePool.Get(modelUUID)
		if err != nil {
			return errors.Trace(err)
		}
		defer st.Release()
		model, err := st.Model()
		if err != nil {
			return errors.Trace(err)
		}

		if model.Life() != state.Alive {
			m.logger.Tracef("skipping upgrade check for dying/dead model %s", modelUUID)
			continue
		}

		cloudspec, err := m.environscloudspecGetter(ctx, names.NewModelTag(modelUUID))
		if err != nil {
			return errors.Trace(err)
		}
		validators := upgradevalidation.ModelValidatorsForControllerModelUpgrade(targetVersion, cloudspec)

		checker := upgradevalidation.NewModelUpgradeCheck(modelUUID, m.statePool, st, model, validators...)
		blockersForModel, err := checker.Validate()
		if err != nil {
			return errors.Annotatef(err, "validating model %q for controller upgrade", model.Name())
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

// shimUpgrader is shim for the state methods that don't have access to
// the context.Context. This allows us to pass it in, until we re-write the
// state layer.
type shimUpgrader struct {
	upgradeService UpgradeService
	ctx            context.Context
}

func (s shimUpgrader) IsUpgrading() (bool, error) {
	return s.upgradeService.IsUpgrading(s.ctx)
}
