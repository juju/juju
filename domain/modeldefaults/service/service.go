// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/schema"

	"github.com/juju/juju/domain/model"
	_ "github.com/juju/juju/domain/model/errors"
	"github.com/juju/juju/domain/modeldefaults"
	"github.com/juju/juju/environs/config"
)

// ModelDefaultsProvider represents a provider that will provide model defaults
// values for a single model. Interfaces of this type are expected to be
// scoped to a predetermined model already.
type ModelDefaultsProvider interface {
	// ModelDefaults returns the default value to use for a specific model. Any
	// errors encountered while process a models defaults will be reported
	// through error.
	ModelDefaults(context.Context) (modeldefaults.Defaults, error)
}

// ModelDefaultsProviderFunc is a func type that implements [ModelDefaultsProvider].
type ModelDefaultsProviderFunc func(context.Context) (modeldefaults.Defaults, error)

// State is the model config state required by this service.
type State interface {
	// ConfigDefaults returns the default configuration values set in Juju.
	ConfigDefaults(context.Context) map[string]any

	// ModelCloudDefaults returns the defaults associated with the model's cloud.
	ModelCloudDefaults(context.Context, model.UUID) (map[string]string, error)

	// ModelCloudRegionDefaults returns the defaults associated with the models
	// set cloud region.
	ModelCloudRegionDefaults(context.Context, model.UUID) (map[string]string, error)

	// ModelProviderConfigSchema returns the providers config schema source based on
	// the cloud set for the model.
	ModelProviderConfigSchema(context.Context, model.UUID) (config.ConfigSchemaSource, error)
}

// Service defines a service for interacting with the underlying default
// configuration options of a model.
type Service struct {
	st State
}

// ModelDefaults implements ModelDefaultsProvider
func (f ModelDefaultsProviderFunc) ModelDefaults(
	ctx context.Context,
) (modeldefaults.Defaults, error) {
	return f(ctx)
}

// ModelDefaults will return the default config values to be used for a model
// and it's config. If no model for uuid is found then a error satisfying
// [github.com/juju/juju/domain/model/errors.NotFound] will be returned.
func (s *Service) ModelDefaults(
	ctx context.Context,
	uuid model.UUID,
) (modeldefaults.Defaults, error) {
	if err := uuid.Validate(); err != nil {
		return modeldefaults.Defaults{}, fmt.Errorf("model uuid: %w", err)
	}

	defaults := modeldefaults.Defaults{}

	jujuDefaults := s.st.ConfigDefaults(ctx)
	for k, v := range jujuDefaults {
		defaults[k] = modeldefaults.DefaultAttributeValue{
			Source: config.JujuDefaultSource,
			Value:  v,
		}
	}

	schemaSource, err := s.st.ModelProviderConfigSchema(ctx, uuid)
	if err != nil && !errors.Is(err, errors.NotFound) {
		return modeldefaults.Defaults{}, errors.Trace(err)
	} else if !errors.Is(err, errors.NotFound) {
		fields := schema.FieldMap(schemaSource.ConfigSchema(), schemaSource.ConfigDefaults())
		coercedAttrs, err := fields.Coerce(defaults, nil)
		if err != nil {
			return modeldefaults.Defaults{}, errors.Trace(err)
		}

		for k, v := range coercedAttrs.(map[string]interface{}) {
			defaults[k] = modeldefaults.DefaultAttributeValue{
				Source: config.JujuDefaultSource,
				Value:  v,
			}
		}
	}

	cloudDefaults, err := s.st.ModelCloudDefaults(ctx, uuid)
	if err != nil {
		return modeldefaults.Defaults{}, fmt.Errorf("getting model %q defaults: %w", uuid, err)
	}

	for k, v := range cloudDefaults {
		defaults[k] = modeldefaults.DefaultAttributeValue{
			Source: config.JujuControllerSource,
			Value:  v,
		}
	}

	cloudRegionDefaults, err := s.st.ModelCloudRegionDefaults(ctx, uuid)
	if err != nil {
		return modeldefaults.Defaults{}, fmt.Errorf("getting model %q defaults: %w", uuid, err)
	}

	for k, v := range cloudRegionDefaults {
		defaults[k] = modeldefaults.DefaultAttributeValue{
			Source: config.JujuRegionSource,
			Value:  v,
		}
	}

	return defaults, nil
}

// ModelDefaultsProvider provides a [ModelDefaultsProviderFunc] scoped to the
// supplied model. This can be used in the construction of
// [github.com/juju/juju/domain/modelconfig/service.Service]. If no model exists
// for the specified UUID then the [ModelDefaultsProviderFunc] will return a
// error that satisfies
// [github.com/juju/juju/domain/model/errors.NotFound].
func (s *Service) ModelDefaultsProvider(
	uuid model.UUID,
) ModelDefaultsProviderFunc {
	return func(ctx context.Context) (modeldefaults.Defaults, error) {
		return s.ModelDefaults(ctx, uuid)
	}
}

// NewService returns a new Service for interacting with model defaults state.
func NewService(st State) *Service {
	return &Service{
		st: st,
	}
}
