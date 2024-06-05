// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelconfig

import (
	"context"
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/loggo/v2"

	"github.com/juju/juju/apiserver/authentication"
	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/rpc/params"
)

// ModelConfigAPI provides the base implementation of the methods.
type ModelConfigAPI struct {
	backend        Backend
	backendService SecretBackendService
	configService  ModelConfigService
	auth           facade.Authorizer
	check          *common.BlockChecker
}

// ModelConfigAPIV3 is currently the latest.
type ModelConfigAPIV3 struct {
	*ModelConfigAPI
}

// NewModelConfigAPI creates a new instance of the ModelConfig Facade.
func NewModelConfigAPI(
	backend Backend,
	backendService SecretBackendService,
	configService ModelConfigService,
	authorizer facade.Authorizer,
) (*ModelConfigAPIV3, error) {
	if !authorizer.AuthClient() {
		return nil, apiservererrors.ErrPerm
	}
	client := &ModelConfigAPI{
		backend:        backend,
		backendService: backendService,
		configService:  configService,
		auth:           authorizer,
		check:          common.NewBlockChecker(backend),
	}
	return &ModelConfigAPIV3{client}, nil
}

func (c *ModelConfigAPI) checkCanWrite() error {
	return c.auth.HasPermission(permission.WriteAccess, c.backend.ModelTag())
}

func (c *ModelConfigAPI) isControllerAdmin() error {
	return c.auth.HasPermission(permission.SuperuserAccess, c.backend.ControllerTag())
}

func (c *ModelConfigAPI) canReadModel() error {
	err := c.auth.HasPermission(permission.SuperuserAccess, c.backend.ControllerTag())
	if err != nil && !errors.Is(err, authentication.ErrorEntityMissingPermission) {
		return errors.Trace(err)
	} else if err == nil {
		return nil
	}

	err = c.auth.HasPermission(permission.AdminAccess, c.backend.ModelTag())
	if err != nil && !errors.Is(err, authentication.ErrorEntityMissingPermission) {
		return errors.Trace(err)
	} else if err == nil {
		return nil
	}

	return c.auth.HasPermission(permission.ReadAccess, c.backend.ModelTag())
}

func (c *ModelConfigAPI) isModelAdmin() (bool, error) {
	err := c.auth.HasPermission(permission.SuperuserAccess, c.backend.ControllerTag())
	if err != nil && !errors.Is(err, authentication.ErrorEntityMissingPermission) {
		return false, errors.Trace(err)
	} else if err == nil {
		return true, nil
	}
	err = c.auth.HasPermission(permission.AdminAccess, c.backend.ModelTag())
	return err == nil, err
}

// ModelGet implements the server-side part of the
// model-config CLI command.
func (c *ModelConfigAPI) ModelGet(ctx context.Context) (params.ModelConfigResults, error) {
	result := params.ModelConfigResults{}
	if err := c.canReadModel(); err != nil {
		return result, errors.Trace(err)
	}

	isAdmin, err := c.isModelAdmin()
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
		// Authorized keys are able to be listed using
		// juju ssh-keys and including them here just
		// clutters everything.
		if attr == config.AuthorizedKeysKey {
			continue
		}

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
	if err := c.checkCanWrite(); err != nil {
		return err
	}

	if err := c.check.ChangeAllowed(ctx); err != nil {
		return errors.Trace(err)
	}
	isAdmin, err := c.isModelAdmin()
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
	err = c.isControllerAdmin()
	if errors.Is(err, authentication.ErrorEntityMissingPermission) {
		isLoggingAdmin = false
	} else if err != nil {
		return errors.Trace(err)
	}

	logValidator := LogTracingValidator(isLoggingAdmin)

	var validationError *config.ValidationError
	err = c.configService.UpdateModelConfig(ctx, args.Config, nil, logValidator)
	if errors.As(err, &validationError) {
		return fmt.Errorf("config key %q %w: %w",
			validationError.InvalidAttrs,
			errors.NotValid,
			validationError.Cause)
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
				Cause:        errors.ConstError("only controller admins can set a model's logging level to TRACE"),
			}
		}
		return cfg, nil
	}
}

// ModelUnset implements the server-side part of the set-model-config CLI command.
func (c *ModelConfigAPI) ModelUnset(ctx context.Context, args params.ModelUnset) error {
	if err := c.checkCanWrite(); err != nil {
		return err
	}
	if err := c.check.ChangeAllowed(ctx); err != nil {
		return errors.Trace(err)
	}

	var validationError config.ValidationError
	err := c.configService.UpdateModelConfig(ctx, nil, args.Keys)
	if errors.As(err, &validationError) {
		return fmt.Errorf("removing config key %q %w: %w",
			validationError.InvalidAttrs,
			errors.NotValid,
			validationError.Cause)
	}

	return err
}

// GetModelConstraints returns the constraints for the model.
func (c *ModelConfigAPI) GetModelConstraints(ctx context.Context) (params.GetConstraintsResults, error) {
	if err := c.canReadModel(); err != nil {
		return params.GetConstraintsResults{}, err
	}

	cons, err := c.backend.ModelConstraints()
	if err != nil {
		return params.GetConstraintsResults{}, err
	}
	return params.GetConstraintsResults{Constraints: cons}, nil
}

// SetModelConstraints sets the constraints for the model.
func (c *ModelConfigAPI) SetModelConstraints(ctx context.Context, args params.SetConstraints) error {
	if err := c.checkCanWrite(); err != nil {
		return err
	}

	if err := c.check.ChangeAllowed(ctx); err != nil {
		return errors.Trace(err)
	}
	return c.backend.SetModelConstraints(args.Constraints)
}

// Sequences returns the model's sequence names and next values.
func (c *ModelConfigAPI) Sequences(ctx context.Context) (params.ModelSequencesResult, error) {
	result := params.ModelSequencesResult{}
	if err := c.canReadModel(); err != nil {
		return result, errors.Trace(err)
	}

	values, err := c.backend.Sequences()
	if err != nil {
		return result, errors.Trace(err)
	}

	result.Sequences = values
	return result, nil
}
