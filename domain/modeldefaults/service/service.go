// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/schema"

	coreerrors "github.com/juju/juju/core/errors"
	coremodel "github.com/juju/juju/core/model"
	_ "github.com/juju/juju/domain/model/errors"
	"github.com/juju/juju/domain/modeldefaults"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/errors"
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

// ModelConfigProviderFunc describes a type that is able to return a
// [environs.ModelConfigProvider] for the specified cloud type. If the no
// model config provider exists for the supplied cloud type then a
// [coreerrors.NotFound] error is returned. If the cloud type provider does not
// support model config then a [coreerrors.NotSupported] error is returned.
type ModelConfigProviderFunc func(string) (environs.ModelConfigProvider, error)

// State is the model config state required by this service.
type State interface {
	// ConfigDefaults returns the default configuration values set in Juju.
	ConfigDefaults(context.Context) map[string]any

	// ModelCloud returns the cloud type used by the model identified by the
	// model uuid. If no model exists for the given uuid then an error
	// [modelerrors.NotFound] is returned.
	ModelCloudType(context.Context, coremodel.UUID) (string, error)

	// ModelCloudDefaults returns the defaults associated with the model's cloud.
	ModelCloudDefaults(context.Context, coremodel.UUID) (map[string]string, error)

	// ModelCloudRegionDefaults returns the defaults associated with the models
	// set cloud region.
	ModelCloudRegionDefaults(context.Context, coremodel.UUID) (map[string]string, error)

	// ModelMetadataDefaults is responsible for providing metadata defaults for a
	// models config. These include things like the models name and uuid.
	// If no model exists for the provided uuid then a [modelerrors.NotFound] error
	// is returned.
	ModelMetadataDefaults(context.Context, coremodel.UUID) (map[string]string, error)
}

// Service defines a service for interacting with the underlying default
// configuration options of a model.
type Service struct {
	modelConfigProviderGetter ModelConfigProviderFunc
	st                        State
}

// ModelDefaults implements ModelDefaultsProvider
func (f ModelDefaultsProviderFunc) ModelDefaults(
	ctx context.Context,
) (modeldefaults.Defaults, error) {
	return f(ctx)
}

// NewService returns a new Service for interacting with model defaults state.
func NewService(
	modelConfigProviderGetter ModelConfigProviderFunc,
	st State,
) *Service {
	return &Service{
		modelConfigProviderGetter: modelConfigProviderGetter,
		st:                        st,
	}
}

// ProviderModelConfigGetter returns a [ModelConfigProviderFunc] for
// retrieving provider based model config  values.
func ProviderModelConfigGetter() ModelConfigProviderFunc {
	return func(cloudType string) (environs.ModelConfigProvider, error) {
		envProvider, err := environs.GlobalProviderRegistry().Provider(cloudType)
		if errors.Is(err, coreerrors.NotFound) {
			return nil, errors.Errorf(
				"no model config provider exists for cloud type %q", cloudType,
			).Add(coreerrors.NotFound)
		}

		modelConfigProvider, supports := envProvider.(environs.ModelConfigProvider)
		if !supports {
			return nil, errors.Errorf(
				"model config provider not supported for cloud type %q", cloudType,
			).Add(coreerrors.NotSupported)
		}

		return modelConfigProvider, nil
	}
}

// providerDefaults is responsible for wrangling and bring together all of the
// model config attributes and their defaults for a provider of a model. There
// are typically two types of defaults a provider has. The first is the defaults
// for the keys the provider extends model config with. These are generally
// provider specific keys and only make sense in the context of the provider.
// The second is defaults the provider can suggest for controller wide
// attributes. Most commonly this is providing a default for the storage to use
// in a model.
func (s *Service) providerDefaults(
	ctx context.Context,
	uuid coremodel.UUID,
) (modeldefaults.Defaults, error) {
	modelCloudType, err := s.st.ModelCloudType(ctx, uuid)
	if err != nil {
		return nil, errors.Errorf(
			"getting model %q cloud type to extract provider model config defaults: %w",
			uuid, err,
		)
	}

	configProvider, err := s.modelConfigProviderGetter(modelCloudType)
	if errors.Is(err, coreerrors.NotFound) {
		return nil, errors.Errorf(
			"getting model %q config provider for cloud %q does not exist",
			uuid, modelCloudType,
		)
	} else if errors.Is(err, coreerrors.NotSupported) {
		// The provider doesn't have anything to contribute to the models
		//defaults.
		return nil, nil
	} else if err != nil {
		return nil, errors.Errorf(
			"getting model %q config provider for cloud %q: %w",
			uuid, modelCloudType, err,
		)
	}

	modelDefaults, err := configProvider.ModelConfigDefaults(ctx)
	if err != nil {
		return nil, errors.Errorf(
			"getting model %q defaults for provider %q: %w",
			uuid, modelCloudType, err,
		)
	}

	fields := schema.FieldMap(configProvider.ConfigSchema(), configProvider.ConfigDefaults())
	coercedAttrs, err := fields.Coerce(map[string]any{}, nil)
	if err != nil {
		return modeldefaults.Defaults{}, errors.Errorf(
			"coercing model %q config provider for cloud %q default schema attributes: %w",
			uuid, modelCloudType, err,
		)
	}

	coercedMap := coercedAttrs.(map[string]any)
	rval := make(modeldefaults.Defaults, len(coercedMap)+len(modelDefaults))

	for k, v := range coercedAttrs.(map[string]interface{}) {
		rval[k] = modeldefaults.DefaultAttributeValue{
			Source: config.JujuDefaultSource,
			Value:  v,
		}
	}

	for k, v := range modelDefaults {
		rval[k] = modeldefaults.DefaultAttributeValue{
			Source: config.JujuDefaultSource,
			Value:  v,
		}
	}

	return rval, nil
}

// ModelDefaults will return the default config values to be used for a model
// and it's config. If no model for uuid is found then a error satisfying
// [github.com/juju/juju/domain/model/errors.NotFound] will be returned.
//
// The order in which to provide defaults is a tricky problem to coerce into one
// place in Juju. Previously this was spread out over many places with no real
// attempt to document which defaults should override another default. This
// function follows the principal that we always start with the hard coded
// defaults defined in Juju and then layer on and overwrite where needed the
// attributes that a user can change. The attributes defaults that a user can
// change are layered in the form of a funnel where we apply the most granular
// specific last. The current order is:
// - Defaults embedded in this Juju version.
// - Provider defaults.
// - Cloud defaults.
// - Cloud region defaults.
// - Model metadata information (this is hardcoded information that can never be changed by the user).
func (s *Service) ModelDefaults(
	ctx context.Context,
	uuid coremodel.UUID,
) (modeldefaults.Defaults, error) {
	if err := uuid.Validate(); err != nil {
		return modeldefaults.Defaults{}, errors.Errorf("model uuid: %w", err)
	}

	defaults := modeldefaults.Defaults{}

	jujuDefaults := s.st.ConfigDefaults(ctx)
	for k, v := range jujuDefaults {
		defaults[k] = modeldefaults.DefaultAttributeValue{
			Source: config.JujuDefaultSource,
			Value:  v,
		}
	}

	providerDefaults, err := s.providerDefaults(ctx, uuid)
	if err != nil {
		return nil, err
	}
	for k, v := range providerDefaults {
		defaults[k] = v
	}

	cloudDefaults, err := s.st.ModelCloudDefaults(ctx, uuid)
	if err != nil {
		return modeldefaults.Defaults{}, errors.Errorf("getting model %q cloud defaults: %w", uuid, err)
	}

	for k, v := range cloudDefaults {
		defaults[k] = modeldefaults.DefaultAttributeValue{
			Source: config.JujuControllerSource,
			Value:  v,
		}
	}

	cloudRegionDefaults, err := s.st.ModelCloudRegionDefaults(ctx, uuid)
	if err != nil {
		return modeldefaults.Defaults{}, errors.Errorf("getting model %q cloud region defaults: %w", uuid, err)
	}

	for k, v := range cloudRegionDefaults {
		defaults[k] = modeldefaults.DefaultAttributeValue{
			Source: config.JujuRegionSource,
			Value:  v,
		}
	}

	// TODO (tlm): We want to remove this eventually. Due to legacy reasons
	// model config currently needs to contain a model's name, type and uuid
	// as config values even though they are not config. They should always be
	// set and never changed by the user. In the new DQlite design the easiest
	// way to keep this behaviour for the moment is to drive them as defaults.
	//
	// Once we can safely remove all reads of these values from a model's config
	// we can remove these default values here.
	metadataDefaults, err := s.st.ModelMetadataDefaults(ctx, uuid)
	if err != nil {
		return modeldefaults.Defaults{}, errors.Errorf("getting model %q metadata defaults: %w", uuid, err)
	}

	for k, v := range metadataDefaults {
		defaults[k] = modeldefaults.DefaultAttributeValue{
			Source:   config.JujuControllerSource,
			Strategy: &modeldefaults.PreferDefaultApplyStrategy{},
			Value:    v,
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
	uuid coremodel.UUID,
) ModelDefaultsProviderFunc {
	return func(ctx context.Context) (modeldefaults.Defaults, error) {
		return s.ModelDefaults(ctx, uuid)
	}
}
