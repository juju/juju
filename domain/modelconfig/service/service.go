// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"fmt"

	"github.com/juju/collections/transform"
	"github.com/juju/errors"

	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/domain/modelconfig/validators"
	"github.com/juju/juju/domain/modeldefaults"
	"github.com/juju/juju/environs/config"
)

// ModelDefaultsProvider is responsible for providing the default config values
// for a model.
type ModelDefaultsProvider interface {
	// ModelDefaults will return the default config values to be used for a model
	// and its config.
	ModelDefaults(context.Context) (modeldefaults.Defaults, error)
}

// Service defines the service for interacting with ModelConfig.
type Service struct {
	defaultsProvider ModelDefaultsProvider
	st               State
	watcherFactory   WatcherFactory
}

// State represents the state entity for accessing and setting per
// model configuration values.
type State interface {
	// AllKeysQuery returns a SQL statement that will return all known model config
	// keys.
	AllKeysQuery() string

	// ModelConfig returns the currently set config for the model.
	ModelConfig(context.Context) (map[string]string, error)

	// ModelConfigHasAttributes returns the set of attributes that model config
	// currently has set out of the list supplied.
	ModelConfigHasAttributes(context.Context, []string) ([]string, error)

	// SetModelConfig is responsible for setting the current model config and
	// overwriting all previously set values even if the config supplied is
	// empty or nil.
	SetModelConfig(context.Context, map[string]string) error

	// UpdateModelConfig is responsible for both inserting, updating and
	// removing model config values for the current model.
	UpdateModelConfig(context.Context, map[string]string, []string) error
}

// WatcherFactory describes methods for creating watchers.
type WatcherFactory interface {
	// NewNamespaceWatcher returns a new namespace watcher
	// for events based on the input change mask.
	NewNamespaceWatcher(string, changestream.ChangeType, string) (watcher.StringsWatcher, error)
}

// NewService creates a new ModelConfig service.
func NewService(
	defaultsProvider ModelDefaultsProvider,
	st State,
	wf WatcherFactory,
) *Service {
	return &Service{
		defaultsProvider: defaultsProvider,
		st:               st,
		watcherFactory:   wf,
	}
}

// ModelConfig returns the current config for the model.
func (s *Service) ModelConfig(ctx context.Context) (*config.Config, error) {
	stConfig, err := s.st.ModelConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("getting model config from state: %w", err)
	}

	altConfig := transform.Map(stConfig, func(k, v string) (string, any) { return k, v })
	return config.New(config.NoDefaults, altConfig)
}

// ModelConfigValues returns the config values for the model and the source of
// the value.
func (s *Service) ModelConfigValues(
	ctx context.Context,
) (config.ConfigValues, error) {
	cfg, err := s.ModelConfig(ctx)
	if err != nil {
		return config.ConfigValues{}, err
	}

	defaults, err := s.defaultsProvider.ModelDefaults(ctx)
	if err != nil {
		return config.ConfigValues{}, fmt.Errorf("getting model defaults: %w", err)
	}

	allAtrs := cfg.AllAttrs()
	if len(allAtrs) == 0 {
		allAtrs = map[string]any{}
		for k, v := range defaults {
			allAtrs[k] = v.Value
		}
	}

	result := make(config.ConfigValues, len(allAtrs))
	for attr, val := range allAtrs {
		isDefault, source := defaults[attr].Has(val)
		if !isDefault {
			source = config.JujuModelConfigSource
		}

		result[attr] = config.ConfigValue{
			Value:  val,
			Source: source,
		}
	}

	return result, nil
}

// buildUpdatedModelConfig is responsible for taking the currently set
// ModelConfig and applying in memory update operations.
func (s *Service) buildUpdatedModelConfig(
	ctx context.Context,
	updates map[string]any,
	removeAttrs []string,
) (*config.Config, *config.Config, error) {
	current, err := s.ModelConfig(ctx)
	if err != nil {
		return nil, current, err
	}

	newConf, err := current.Remove(removeAttrs)
	if err != nil {
		return newConf, current, fmt.Errorf("building new model config with removed attributes: %w", err)
	}

	newConf, err = newConf.Apply(updates)
	if err != nil {
		return newConf, current, fmt.Errorf("building new model config with removed attributes: %w", err)
	}

	return newConf, current, nil
}

// reconcileRemovedAttributes will take a set of attributes to remove from the
// model config and figure out if there exists a model default for the
// attribute. If a model default exists then a set of updates will be provided
// to instead change the attribute to the model default. This function will also
// check that the removed attributes first exist in the model's config otherwise
// we risk bringing in model defaults that were not previously set for the model
// config.
func (s *Service) reconcileRemovedAttributes(
	ctx context.Context,
	removeAttrs []string,
) (map[string]any, error) {
	updates := map[string]any{}
	hasAttrs, err := s.st.ModelConfigHasAttributes(ctx, removeAttrs)
	if err != nil {
		return updates, fmt.Errorf("determining model config has attributes: %w", err)
	}

	defaults, err := s.defaultsProvider.ModelDefaults(ctx)
	if err != nil {
		return updates, fmt.Errorf("getting model defaults for config attribute removal: %w", err)
	}

	for _, attr := range hasAttrs {
		if val := defaults[attr].Value; val != nil {
			updates[attr] = val
		}
	}

	return updates, nil
}

// SetModelConfig will remove any existing model config for the model and
// replace with the new config provided. The new config will also be hydrated
// with any model default attributes that have not been set on the config.
func (s *Service) SetModelConfig(
	ctx context.Context,
	cfg *config.Config,
) error {
	attrs := cfg.AllAttrs()
	defaults, err := s.defaultsProvider.ModelDefaults(ctx)
	if err != nil {
		return fmt.Errorf("getting model defaults: %w", err)
	}

	for k, v := range defaults {
		applyVal := v.ApplyStrategy(attrs[k])
		if applyVal != nil {
			attrs[k] = applyVal
		}
	}

	cfg, err = config.New(config.NoDefaults, attrs)
	if err != nil {
		return fmt.Errorf("constructing new model config with model defaults: %w", err)
	}

	_, err = config.ModelValidator().Validate(cfg, nil)
	if err != nil {
		return fmt.Errorf("validating model config to set for model: %w", err)
	}

	rawCfg, err := CoerceConfigForStorage(cfg.AllAttrs())
	if err != nil {
		return fmt.Errorf("coercing model config for storage: %w", err)
	}

	return s.st.SetModelConfig(ctx, rawCfg)
}

// UpdateModelConfig takes a set of updated and removed attributes to apply.
// Removed attributes are replaced with their model default values should they
// exist. All model config updates are validated against the currently set
// model config. The model config is ran through several validation steps before
// being persisted. If an error occurs during validation then a
// config.ValidationError is returned. The caller can also optionally pass in
// additional config.Validators to be run.
func (s *Service) UpdateModelConfig(
	ctx context.Context,
	updateAttrs map[string]any,
	removeAttrs []string,
	additionalValidators ...config.Validator,
) error {
	// noop with no updates or removals to perform.
	if len(updateAttrs) == 0 && len(removeAttrs) == 0 {
		return nil
	}

	updates, err := s.reconcileRemovedAttributes(ctx, removeAttrs)
	if err != nil {
		return errors.Trace(err)
	}

	// It's important here that we apply the user updates over the top of the
	// calculated ones. This way we always take the user's supplied key value
	// over defaults.
	for k, v := range updateAttrs {
		updates[k] = v
	}

	newCfg, currCfg, err := s.buildUpdatedModelConfig(ctx, updates, removeAttrs)
	if err != nil {
		return fmt.Errorf("making updated model configuration: %w", err)
	}

	_, err = s.updateModelConfigValidator().Validate(newCfg, currCfg)
	if err != nil {
		return fmt.Errorf("validating updated model configuration: %w", err)
	}

	rawCfg, err := CoerceConfigForStorage(updateAttrs)
	if err != nil {
		return fmt.Errorf("coercing new configuration for persistence: %w", err)
	}

	err = s.st.UpdateModelConfig(ctx, rawCfg, removeAttrs)
	if err != nil {
		return fmt.Errorf("updating model config: %w", err)
	}
	return nil
}

// Watch returns a watcher that returns keys for any changes to model
// config.
func (s *Service) Watch() (watcher.StringsWatcher, error) {
	return s.watcherFactory.NewNamespaceWatcher("model_config", changestream.All, s.st.AllKeysQuery())
}

// dummySecretsBackendProvider implements validators.SecretBackendProvider and
// always returns true.
// TODO (tlm): These needs to be swapped out with an actual checker when we have
// secrets in dqlite
type dummySecretsBackendProvider struct{}

// dummySpaceProvider implements validators.SpaceProvider and always returns true.
// TODO (tlm): These needs to be swapped out with an actual checker when we have
// spaces in dqlite
type dummySpaceProvider struct{}

// HasSecretsBackend implements validators.SecretBackendProvider
func (_ *dummySecretsBackendProvider) HasSecretsBackend(_ string) (bool, error) {
	return true, nil
}

// HasSpace implements validators.SpaceProvider
func (_ *dummySpaceProvider) HasSpace(_ string) (bool, error) {
	return true, nil
}

func (s *Service) updateModelConfigValidator(
	additional ...config.Validator,
) config.Validator {
	agg := &config.AggregateValidator{
		Validators: []config.Validator{
			config.ModelValidator(),
			validators.CharmhubURLChange(),
			validators.SpaceChecker(&dummySpaceProvider{}),
			validators.SecretBackendChecker(&dummySecretsBackendProvider{}),
		},
	}
	agg.Validators = append(agg.Validators, additional...)
	return agg
}
