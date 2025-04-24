// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelconfig

import (
	"context"
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/loggo/v2"
	"github.com/juju/names/v6"

	"github.com/juju/juju/apiserver/authentication"
	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/permission"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	modelerrors "github.com/juju/juju/domain/model/errors"
	networkerrors "github.com/juju/juju/domain/network/errors"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/rpc/params"
)

// ModelConfigAPI provides the base implementation of the methods.
type ModelConfigAPI struct {
	backend                   Backend
	controllerUUID            string
	modelSecretBackendService ModelSecretBackendService
	configService             ModelConfigService
	modelSericve              ModelService
	auth                      facade.Authorizer
	check                     *common.BlockChecker

	modelUUID coremodel.UUID
}

// ModelConfigAPIV3 is currently the latest.
type ModelConfigAPIV3 struct {
	*ModelConfigAPI
}

// NewModelConfigAPI creates a new instance of the ModelConfig Facade.
func NewModelConfigAPI(
	modelUUID coremodel.UUID,
	controllerUUID string,
	backend Backend,
	modelSecretBackendService ModelSecretBackendService,
	configService ModelConfigService,
	modelSericve ModelService,
	authorizer facade.Authorizer,
	blockCommandService common.BlockCommandService,
) (*ModelConfigAPI, error) {
	if !authorizer.AuthClient() {
		return nil, apiservererrors.ErrPerm
	}

	return &ModelConfigAPI{
		modelUUID:                 modelUUID,
		backend:                   backend,
		controllerUUID:            controllerUUID,
		modelSecretBackendService: modelSecretBackendService,
		configService:             configService,
		modelSericve:              modelSericve,
		auth:                      authorizer,
		check:                     common.NewBlockChecker(blockCommandService),
	}, nil
}

func (c *ModelConfigAPI) checkCanWrite(ctx context.Context) error {
	return c.auth.HasPermission(ctx, permission.WriteAccess, names.NewModelTag(c.modelUUID.String()))
}

func (c *ModelConfigAPI) isControllerAdmin(ctx context.Context) error {
	return c.auth.HasPermission(ctx, permission.SuperuserAccess, names.NewControllerTag(c.controllerUUID))
}

func (c *ModelConfigAPI) canReadModel(ctx context.Context) error {
	err := c.auth.HasPermission(ctx, permission.SuperuserAccess, names.NewControllerTag(c.controllerUUID))
	if err != nil && !errors.Is(err, authentication.ErrorEntityMissingPermission) {
		return errors.Trace(err)
	} else if err == nil {
		return nil
	}

	modelTag := names.NewModelTag(c.modelUUID.String())

	err = c.auth.HasPermission(ctx, permission.AdminAccess, modelTag)
	if err != nil && !errors.Is(err, authentication.ErrorEntityMissingPermission) {
		return errors.Trace(err)
	} else if err == nil {
		return nil
	}

	return c.auth.HasPermission(ctx, permission.ReadAccess, modelTag)
}

func (c *ModelConfigAPI) isModelAdmin(ctx context.Context) (bool, error) {
	err := c.auth.HasPermission(ctx, permission.SuperuserAccess, names.NewControllerTag(c.controllerUUID))
	if err != nil && !errors.Is(err, authentication.ErrorEntityMissingPermission) {
		return false, errors.Trace(err)
	} else if err == nil {
		return true, nil
	}
	err = c.auth.HasPermission(ctx, permission.AdminAccess, names.NewModelTag(c.modelUUID.String()))
	return err == nil, err
}

// ModelGet implements the server-side part of the
// model-config CLI command.
func (c *ModelConfigAPI) ModelGet(ctx context.Context) (params.ModelConfigResults, error) {
	result := params.ModelConfigResults{}
	if err := c.canReadModel(ctx); err != nil {
		return result, errors.Trace(err)
	}

	isAdmin, err := c.isModelAdmin(ctx)
	if err != nil && !errors.Is(err, authentication.ErrorEntityMissingPermission) {
		return result, errors.Trace(err)
	}

	defaultSchema, err := config.Schema(nil)
	if err != nil {
		return result, errors.Trace(err)
	}

	values, err := c.configService.ModelConfigValues(ctx)
	if err != nil {
		return result, errors.Trace(err)
	}

	result.Config = make(map[string]params.ConfigValue)
	for attr, val := range values {
		// Only admins get to see attributes marked as secret.
		if attr, ok := defaultSchema[attr]; ok && attr.Secret && !isAdmin {
			continue
		}

		result.Config[attr] = params.ConfigValue{
			Value:  val.Value,
			Source: val.Source,
		}
	}

	return result, nil
}

// ModelSet implements the server-side part of the
// set-model-config CLI command.
func (c *ModelConfigAPI) ModelSet(ctx context.Context, args params.ModelSet) error {
	if err := c.checkCanWrite(ctx); err != nil {
		return err
	}

	if err := c.check.ChangeAllowed(ctx); err != nil {
		return errors.Trace(err)
	}
	isAdmin, err := c.isModelAdmin(ctx)
	if err != nil && !errors.Is(err, authentication.ErrorEntityMissingPermission) {
		return errors.Trace(err)
	}
	defaultSchema, err := config.Schema(nil)
	if err != nil {
		return errors.Trace(err)
	}
	// Only admins get to set attributes marked as secret.
	for attr := range args.Config {
		if attr, ok := defaultSchema[attr]; ok && attr.Secret && !isAdmin {
			return apiservererrors.ErrPerm
		}
	}

	isLoggingAdmin := true
	err = c.isControllerAdmin(ctx)
	if errors.Is(err, authentication.ErrorEntityMissingPermission) {
		isLoggingAdmin = false
	} else if err != nil {
		return errors.Trace(err)
	}

	logValidator := LogTracingValidator(isLoggingAdmin)

	var validationError *config.ValidationError
	err = c.configService.UpdateModelConfig(ctx, args.Config, nil, logValidator)
	if errors.As(err, &validationError) {
		return fmt.Errorf("config key %q %w: %s",
			validationError.InvalidAttrs,
			errors.NotValid,
			validationError.Reason)
	}

	return err
}

// LogTracingValidator is a logging config validator that checks if a logging
// change is being requested, specifically that of trace and to see if the
// requesting user has the required permission for the change.
func LogTracingValidator(isAdmin bool) config.ValidatorFunc {
	return func(ctx context.Context, cfg, old *config.Config) (*config.Config, error) {
		spec := cfg.LoggingConfig()
		oldSpec := old.LoggingConfig()

		// No change so no need to keep going on with the check.
		if spec == oldSpec {
			return cfg, nil
		}

		logCfg, err := loggo.ParseConfigString(spec)
		if err != nil {
			return cfg, fmt.Errorf("validating logging configuration for model config: %w", err)
		}

		// Does at least one package have TRACE level logging requested.
		haveTrace := false
		for _, level := range logCfg {
			haveTrace = level == loggo.TRACE
			if haveTrace {
				break
			}
		}
		// No TRACE level requested, so no need to check for admin.
		if !haveTrace {
			return cfg, nil
		}

		if !isAdmin {
			return cfg, &config.ValidationError{
				InvalidAttrs: []string{config.LoggingConfigKey},
				Reason:       "only controller admins can set a model's logging level to TRACE",
			}
		}
		return cfg, nil
	}
}

// ModelUnset implements the server-side part of the set-model-config CLI command.
func (c *ModelConfigAPI) ModelUnset(ctx context.Context, args params.ModelUnset) error {
	if err := c.checkCanWrite(ctx); err != nil {
		return err
	}
	if err := c.check.ChangeAllowed(ctx); err != nil {
		return errors.Trace(err)
	}

	var validationError config.ValidationError
	err := c.configService.UpdateModelConfig(ctx, nil, args.Keys)
	if errors.As(err, &validationError) {
		return fmt.Errorf("removing config key %q %w: %s",
			validationError.InvalidAttrs,
			errors.NotValid,
			validationError.Reason)
	}

	return err
}

// GetModelConstraints returns the constraints for the model.
func (c *ModelConfigAPI) GetModelConstraints(ctx context.Context) (params.GetConstraintsResults, error) {
	if err := c.canReadModel(ctx); err != nil {
		return params.GetConstraintsResults{}, err
	}

	cons, err := c.modelSericve.GetModelConstraints(ctx)
	if errors.Is(err, modelerrors.NotFound) {
		return params.GetConstraintsResults{}, apiservererrors.ParamsErrorf(
			params.CodeModelNotFound,
			"model %q not found",
			c.modelUUID,
		)
	} else if err != nil {
		return params.GetConstraintsResults{}, err
	}
	return params.GetConstraintsResults{Constraints: cons}, nil
}

// SetModelConstraints sets the constraints for the model.
func (c *ModelConfigAPI) SetModelConstraints(ctx context.Context, args params.SetConstraints) error {
	if err := c.checkCanWrite(ctx); err != nil {
		return err
	}

	if err := c.check.ChangeAllowed(ctx); err != nil {
		return errors.Trace(err)
	}
	err := c.modelSericve.SetModelConstraints(ctx, args.Constraints)
	if errors.Is(err, modelerrors.NotFound) {
		return apiservererrors.ParamsErrorf(
			params.CodeModelNotFound,
			"model %q not found",
			c.modelUUID,
		)
	}
	if errors.Is(err, networkerrors.SpaceNotFound) {
		return apiservererrors.ParamsErrorf(
			params.CodeNotFound,
			"space not found for model constraint %q: %v",
			c.modelUUID,
			err,
		)
	}
	if errors.Is(err, machineerrors.InvalidContainerType) {
		return apiservererrors.ParamsErrorf(
			params.CodeNotValid,
			"invalid container type for model constraint %q",
			c.modelUUID,
		)
	}
	return errors.Trace(err)
}

// Sequences returns the model's sequence names and next values.
func (c *ModelConfigAPI) Sequences(ctx context.Context) (params.ModelSequencesResult, error) {
	result := params.ModelSequencesResult{}
	if err := c.canReadModel(ctx); err != nil {
		return result, errors.Trace(err)
	}

	values, err := c.backend.Sequences()
	if err != nil {
		return result, errors.Trace(err)
	}

	result.Sequences = values
	return result, nil
}

// GetModelSecretBackend isn't implemented in the ModelConfigAPIV3 facade.
func (s *ModelConfigAPIV3) GetModelSecretBackend(struct{}) {}

// GetModelSecretBackend returns the secret backend for the model,
// returning an error satisfying [authentication.ErrorEntityMissingPermission] if the user does not have read access to the model,
// returning [params.CodeModelNotFound] if the model does not exist.
func (s *ModelConfigAPI) GetModelSecretBackend(ctx context.Context) (params.StringResult, error) {
	result := params.StringResult{}
	if err := s.auth.HasPermission(ctx, permission.ReadAccess, names.NewModelTag(s.modelUUID.String())); err != nil {
		return result, errors.Trace(err)
	}

	name, err := s.modelSecretBackendService.GetModelSecretBackend(ctx)
	if err != nil {
		result.Error = apiservererrors.ServerError(err)
	} else {
		result.Result = name
	}
	return result, nil
}

// SetModelSecretBackend isn't implemented in the ModelConfigAPIV3 facade.
func (s *ModelConfigAPIV3) SetModelSecretBackend(_, _ struct{}) {}

// SetModelSecretBackend sets the secret backend for the model,
// returning an error satisfying [authentication.ErrorEntityMissingPermission] if the user does not have write access to the model,
// returning [params.CodeModelNotFound] if the model does not exist,
// returning [params.CodeSecretBackendNotFound] if the secret backend does not exist,
// returning [params.CodeSecretBackendNotValid] if the secret backend name is not valid.
func (s *ModelConfigAPI) SetModelSecretBackend(ctx context.Context, arg params.SetModelSecretBackendArg) (params.ErrorResult, error) {
	if err := s.auth.HasPermission(ctx, permission.WriteAccess, names.NewModelTag(s.modelUUID.String())); err != nil {
		return params.ErrorResult{}, errors.Trace(err)
	}
	err := s.modelSecretBackendService.SetModelSecretBackend(ctx, arg.SecretBackendName)
	return params.ErrorResult{Error: apiservererrors.ServerError(err)}, nil
}
