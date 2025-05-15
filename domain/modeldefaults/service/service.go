// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/collections/transform"
	"github.com/juju/schema"

	"github.com/juju/juju/core/cloud"
	coreerrors "github.com/juju/juju/core/errors"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/trace"
	clouderrors "github.com/juju/juju/domain/cloud/errors"
	_ "github.com/juju/juju/domain/model/errors"
	modelconfigservice "github.com/juju/juju/domain/modelconfig/service"
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
	// GetModelCloudUUID returns the cloud UUID for the given model.
	// If the model is not found, an error specifying [modelerrors.NotFound] is returned.
	GetModelCloudUUID(context.Context, coremodel.UUID) (cloud.UUID, error)

	// GetCloudUUID returns the cloud UUID and region for the given cloud name.
	// If the cloud is not found, an error specifying [clouderrors.NotFound] is returned.
	GetCloudUUID(context.Context, string) (cloud.UUID, error)

	// UpdateCloudDefaults is responsible for updating default config values for a
	// cloud. This function will allow the addition and updating of attributes.
	// If the cloud doesn't exist, an error satisfying [clouderrors.NotFound]
	// is returned.
	UpdateCloudDefaults(ctx context.Context, cloudUID cloud.UUID, attrs map[string]string) error

	// DeleteCloudDefaults will delete the specified default config keys from
	// the cloud if they exist. If the cloud does not exist an error satisfying
	// [clouderrors.NotFound] will be returned.
	DeleteCloudDefaults(ctx context.Context, cloudUID cloud.UUID, attrs []string) error

	// UpdateCloudRegionDefaults is responsible for updating default config values
	// for a cloud region. This function will allow the addition and updating of
	// attributes. If the cloud is not found an error satisfying [clouderrors.NotFound]
	// is returned. If the region is not found, am error satisfying [errors.NotFound]
	// is returned.
	UpdateCloudRegionDefaults(ctx context.Context, cloudUID cloud.UUID, regionName string, attrs map[string]string) error

	// DeleteCloudRegionDefaults deletes the specified default config keys for
	// the given cloud region. It returns an error satisfying
	// [clouderrors.NotFound] if the cloud region doesn't exist.
	DeleteCloudRegionDefaults(ctx context.Context, cloudUID cloud.UUID, regionName string, attrs []string) error

	// ConfigDefaults returns the default configuration values set in Juju.
	ConfigDefaults(context.Context) map[string]any

	// CloudDefaults returns the defaults associated with the given cloud. If
	// no defaults are found then an empty map will be returned with a nil
	// error. If no cloud exists for the given id an error satisfying
	// [clouderrors.NotFound] will be returned.
	CloudDefaults(context.Context, cloud.UUID) (map[string]string, error)

	// ModelCloudRegionDefaults returns the defaults associated with the model's cloud region.
	// It returns an error satisfying [modelerrors.NotFound] if the model doesn't exist.
	ModelCloudRegionDefaults(ctx context.Context, uuid coremodel.UUID) (map[string]string, error)

	// CloudAllRegionDefaults returns the defaults associated with all of the
	// regions for the specified cloud. The result is a map of region name
	// key values, keyed on the name of the region.
	// If no defaults are found then an empty map will be returned with nil error.
	// Note this will not include the defaults set on the cloud itself but
	// just that of its regions.
	CloudAllRegionDefaults(
		ctx context.Context,
		cloudUUID cloud.UUID,
	) (map[string]map[string]string, error)

	// ModelMetadataDefaults is responsible for providing metadata defaults for
	// a model's config. These include things like the model's name and uuid.
	// If no model exists for the provided uuid then a [modelerrors.NotFound]
	// error is returned.
	// Deprecated: this is only to support legacy callers.
	ModelMetadataDefaults(context.Context, coremodel.UUID) (map[string]string, error)

	// CloudType returns the cloud type of the cloud.
	// If no cloud exists for the given uuid then an error
	// satisfying [clouderrors.NotFound] is returned.
	CloudType(context.Context, cloud.UUID) (string, error)
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

// ProviderDefaults is responsible for wrangling and bringing together the model
// defaults that should be applied to model from a given provider. There are
// typically two types of defaults a provider has. The first is the defaults for
// the keys the provider extends model config with. These are generally provider
// specific keys and only make sense in the context of the provider. The second
// is defaults the provider can suggest for controller wide attributes. Most
// commonly this is providing a default for the storage to use in a model.
func ProviderDefaults(
	ctx context.Context,
	cloudType string,
	providerGetter ModelConfigProviderFunc,
) (map[string]any, error) {
	configProvider, err := providerGetter(cloudType)
	if errors.Is(err, coreerrors.NotFound) {
		return nil, errors.Errorf(
			"getting model config provider, provider for cloud type %q does not exist",
			cloudType,
		)
	} else if errors.Is(err, coreerrors.NotSupported) {
		// The provider doesn't have anything to contribute.
		return nil, nil
	} else if err != nil {
		return nil, errors.Errorf(
			"getting model config provider for cloud type %q: %w",
			cloudType, err,
		)
	}

	modelDefaults, err := configProvider.ModelConfigDefaults(ctx)
	if err != nil {
		return nil, errors.Errorf(
			"getting model defaults for provider of cloud type %q: %w",
			cloudType, err,
		)
	}

	fields := schema.FieldMap(configProvider.ConfigSchema(), configProvider.ConfigDefaults())
	coercedAttrs, err := fields.Coerce(map[string]any{}, nil)
	if err != nil {
		return nil, errors.Errorf(
			"coercing model config provider for cloud type %q default schema attributes: %w",
			cloudType, err,
		)
	}

	coercedMap := coercedAttrs.(map[string]any)
	rval := make(map[string]any, len(coercedMap)+len(modelDefaults))

	for k, v := range coercedAttrs.(map[string]interface{}) {
		rval[k] = v
	}

	for k, v := range modelDefaults {
		rval[k] = v
	}

	return rval, nil
}

// CloudDefaults returns the default attribute details for a specified cloud.
// It returns an error satisfying [clouderrors.NotFound] if the cloud doesn't
// exist.
func (s *Service) CloudDefaults(
	ctx context.Context,
	cloudName string,
) (modeldefaults.ModelDefaultAttributes, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	cloudUUID, err := s.st.GetCloudUUID(ctx, cloudName)
	if errors.Is(err, clouderrors.NotFound) {
		return nil, errors.Errorf(
			"cloud %q does not exist", cloudName,
		).Add(clouderrors.NotFound)
	} else if err != nil {
		return nil, errors.Errorf("getting cloud %q uuid: %w", cloudName, err)
	}

	cloudType, err := s.st.CloudType(ctx, cloudUUID)
	if err != nil {
		return nil, errors.Errorf(
			"getting cloud %q type to extract provider model config defaults: %w",
			cloudUUID, err,
		)
	}

	cloudDefaults, err := s.cloudDefaults(ctx, cloudUUID, cloudType)
	if err != nil {
		return nil, errors.Errorf("getting cloud defaults for cloud %q: %w", cloudName, err)
	}

	regionDefaults, err := s.cloudAllRegionDefaults(ctx, cloudUUID, cloudType)
	if err != nil {
		return nil, errors.Errorf("getting cloud region defaults for cloud %q: %w", cloudUUID, err)
	}

	defaults := modeldefaults.ModelDefaultAttributes{}
	for k, v := range cloudDefaults {
		defaults[k] = modeldefaults.AttributeDefaultValues{
			Default:    v.Default,
			Controller: v.Controller,
		}
	}

	// Transform the region defaults keys on region name into
	// a slice of region default values.
	for regionName, regionAttr := range regionDefaults {
		for k := range regionAttr {
			val := defaults[k]
			val.Regions = append(val.Regions, modeldefaults.RegionDefaultValue{
				Name:  regionName,
				Value: regionAttr[k],
			})
			defaults[k] = val
		}
	}
	return defaults, nil
}

// UpdateCloudDefaults saves the specified default attribute details for a
// cloud. It returns an error satisfying [clouderrors.NotFound] if the cloud
// doesn't exist.
func (s *Service) UpdateCloudDefaults(
	ctx context.Context,
	cloudName string,
	updateAttrs map[string]any,
) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	cloudUUID, err := s.st.GetCloudUUID(ctx, cloudName)
	if errors.Is(err, clouderrors.NotFound) {
		return errors.Errorf(
			"cloud %q does not exist", cloudName,
		).Add(clouderrors.NotFound)
	} else if err != nil {
		return errors.Errorf("getting cloud UUID for cloud %q: %w", cloudName, err)
	}

	if len(updateAttrs) == 0 {
		return nil
	}

	strAttrs, err := modelconfigservice.CoerceConfigForStorage(updateAttrs)
	if err != nil {
		return errors.Errorf("coercing cloud %q default values for storage: %w", cloudName, err)
	}
	return s.st.UpdateCloudDefaults(ctx, cloudUUID, strAttrs)
}

// UpdateCloudRegionDefaults saves the specified default attribute details for a
// cloud region. It returns an error satisfying [clouderrors.NotFound] if the
// cloud doesn't exist.
func (s *Service) UpdateCloudRegionDefaults(
	ctx context.Context,
	cloudName string,
	regionName string,
	updateAttrs map[string]any,
) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	cloudUUID, err := s.st.GetCloudUUID(ctx, cloudName)
	if errors.Is(err, clouderrors.NotFound) {
		return errors.Errorf(
			"cloud %q region %q does not exist", cloudName, regionName,
		).Add(clouderrors.NotFound)
	} else if err != nil {
		return errors.Errorf("getting cloud UUID for cloud %q: %w", cloudName, err)
	}

	strAttrs, err := modelconfigservice.CoerceConfigForStorage(updateAttrs)
	if err != nil {
		return errors.Errorf("coercing cloud %q default values for storage: %w", cloudName, err)
	}

	err = s.st.UpdateCloudRegionDefaults(ctx, cloudUUID, regionName, strAttrs)
	if errors.Is(err, clouderrors.NotFound) {
		return errors.Errorf(
			"cloud %q region %q does not exist", cloudName, regionName,
		).Add(clouderrors.NotFound)
	}
	return err
}

// RemoveCloudDefaults deletes the specified default attribute details for a
// cloud. It returns an error satisfying [clouderrors.NotFound] if the cloud
// doesn't exist.
func (s *Service) RemoveCloudDefaults(
	ctx context.Context,
	cloudName string,
	removeAttrs []string,
) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	cloudUUID, err := s.st.GetCloudUUID(ctx, cloudName)
	if errors.Is(err, clouderrors.NotFound) {
		return errors.Errorf(
			"cloud %q does not exist", cloudName,
		).Add(clouderrors.NotFound)
	} else if err != nil {
		return errors.Errorf("getting cloud UUID for cloud %q: %w", cloudName, err)
	}

	if len(removeAttrs) == 0 {
		return nil
	}

	return s.st.DeleteCloudDefaults(ctx, cloudUUID, removeAttrs)
}

// RemoveCloudRegionDefaults deletes the specified default attribute details for
// a cloud region. It returns an error satisfying [clouderrors.NotFound] if the
// cloud or region doesn't exist.
func (s *Service) RemoveCloudRegionDefaults(
	ctx context.Context,
	cloudName,
	regionName string,
	removeAttrs []string,
) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	cloudUUID, err := s.st.GetCloudUUID(ctx, cloudName)
	if errors.Is(err, clouderrors.NotFound) {
		return errors.Errorf(
			"cloud %q region %q does not exist", cloudName, regionName,
		).Add(clouderrors.NotFound)
	} else if err != nil {
		return errors.Errorf("getting cloud UUID for cloud %q: %w", cloudName, err)
	}

	return s.st.DeleteCloudRegionDefaults(ctx, cloudUUID, regionName, removeAttrs)
}

// ModelDefaults will return the default config values to be used for a model
// and its config. If no model for uuid is found then a error satisfying
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
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := uuid.Validate(); err != nil {
		return modeldefaults.Defaults{}, errors.Errorf("model uuid: %w", err)
	}
	cloudUUID, err := s.st.GetModelCloudUUID(ctx, uuid)
	if err != nil {
		return modeldefaults.Defaults{}, errors.Errorf("getting cloud UUID for model %q: %w", uuid, err)
	}
	cloudType, err := s.st.CloudType(ctx, cloudUUID)
	if err != nil {
		return nil, errors.Errorf(
			"getting %q cloud type to extract provider model config defaults: %w",
			cloudUUID, err,
		)
	}

	defaults, err := s.cloudDefaults(ctx, cloudUUID, cloudType)
	if err != nil {
		return modeldefaults.Defaults{}, errors.Errorf("getting cloud defaults for model %q with cloud %q: %w", uuid, cloudUUID, err)
	}

	regionDefaults, err := s.modelCloudRegionDefaults(
		ctx,
		uuid,
		cloudType,
	)
	if err != nil {
		return modeldefaults.Defaults{}, errors.Errorf(
			"getting cloud region default for model %q with cloud %q: %w", uuid, cloudUUID, err)
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
		val := defaults[k]
		val.Controller = v
		defaults[k] = val
	}

	result := modeldefaults.Defaults{}
	for k, v := range defaults {
		val := modeldefaults.DefaultAttributeValue{
			Default:    v.Default,
			Controller: v.Controller,
			Region:     regionDefaults[k],
		}
		result[k] = val
	}
	return result, nil
}

// cloudDefaults will return the default config values which have been
// specified for a cloud and its regions. The string values from the
// database are coerced into the types specified by the cloud's config schema.
func (s *Service) cloudDefaults(
	ctx context.Context,
	cloudUUID cloud.UUID,
	cloudType string,
) (modeldefaults.ModelCloudDefaultAttributes, error) {
	if err := cloudUUID.Validate(); err != nil {
		return modeldefaults.ModelCloudDefaultAttributes{}, errors.Errorf("cloud uuid: %w", err)
	}

	defaults := modeldefaults.ModelCloudDefaultAttributes{}

	jujuDefaults := s.st.ConfigDefaults(ctx)
	for k, v := range jujuDefaults {
		defaults[k] = modeldefaults.CloudDefaultValues{
			Default: v,
		}
	}

	providerDefaults, err := ProviderDefaults(ctx, cloudType, s.modelConfigProviderGetter)
	if err != nil {
		return nil, errors.Errorf(
			"getting cloud %q provider defaults: %w", cloudUUID, err,
		)
	}

	for k, v := range providerDefaults {
		defaults[k] = modeldefaults.CloudDefaultValues{
			Default: v,
		}
	}

	// Process the cloud defaults.
	dbCloudDefaults, err := s.st.CloudDefaults(ctx, cloudUUID)
	if err != nil {
		return modeldefaults.ModelCloudDefaultAttributes{}, errors.Errorf("getting model %q cloud defaults: %w", cloudUUID, err)
	}

	coercedCloudDefaults, err := coerceDefaultsToSchema(dbCloudDefaults, cloudType, s.modelConfigProviderGetter)
	if err != nil {
		return modeldefaults.ModelCloudDefaultAttributes{}, err
	}
	for k := range dbCloudDefaults {
		val := defaults[k]
		val.Controller = coercedCloudDefaults[k]
		defaults[k] = val
	}
	return defaults, nil
}

// cloudAllRegionDefaults returns the defaults for all
// cloud regions for the cloud.
func (s *Service) cloudAllRegionDefaults(
	ctx context.Context,
	cloudUUID cloud.UUID,
	cloudType string,
) (map[string]map[string]any, error) {
	dbCloudRegionDefaults, err := s.st.CloudAllRegionDefaults(ctx, cloudUUID)
	if err != nil {
		return nil, errors.Errorf("getting model %q cloud region defaults: %w", cloudUUID, err)
	}

	cloudRegionDefaults := make(map[string]map[string]any)
	// Coerce the cloud region config defaults if a cloud config schema has been found.
	for regionName, regionAttr := range dbCloudRegionDefaults {
		coercedAttrs, err := coerceDefaultsToSchema(regionAttr, cloudType, s.modelConfigProviderGetter)
		if err != nil {
			return nil, errors.Errorf(
				"coercing cloud %q region %q config: %w",
				cloudUUID, regionName, err,
			)
		}
		cloudRegionDefaults[regionName] = coercedAttrs
	}
	return cloudRegionDefaults, nil
}

// modelCloudRegionDefaults returns the defaults for the model's cloud region.
func (s *Service) modelCloudRegionDefaults(
	ctx context.Context,
	modelUUID coremodel.UUID,
	cloudType string,
) (map[string]any, error) {
	dbCloudRegionDefaults, err := s.st.ModelCloudRegionDefaults(ctx, modelUUID)
	if err != nil {
		return nil, errors.Errorf("getting model %q cloud region defaults: %w", modelUUID, err)
	}

	// Coerce the cloud region config defaults if a cloud config schema has been found.
	coercedAttrs, err := coerceDefaultsToSchema(dbCloudRegionDefaults, cloudType, s.modelConfigProviderGetter)
	if err != nil {
		return nil, err
	}
	return coercedAttrs, nil
}

// coerceDefaultsToSchema is responsible for taking string representations of model
// default values from state and coercing them into the correct types according
// to the Juju config schema.
func coerceDefaultsToSchema(
	strConfig map[string]string,
	cloudType string,
	providerGetter ModelConfigProviderFunc,
) (map[string]any, error) {
	configProvider, err := providerGetter(cloudType)
	if errors.Is(err, coreerrors.NotFound) {
		return nil, errors.Errorf(
			"getting model config provider, provider for cloud type %q does not exist",
			cloudType,
		)
	} else if err != nil && !errors.Is(err, coreerrors.NotSupported) {
		return nil, errors.Errorf(
			"getting model config provider for cloud type %q: %w",
			cloudType, err,
		)
	}

	providerFieldMap := schema.FieldMap(nil, nil)
	if configProvider != nil {
		providerSchema := configProvider.ConfigSchema()

		// We are building a set of defaults here for each key that exists in
		// the provider's schema set to [schema.Omit]. The reason for this is
		// that [schema.FieldMap.Coerce] will try and apply defaults for keys
		// that don't exist in the input.
		//
		// We don't want this to happen here. The purpose of this function is to
		// fundamentally coerce the type we store the value in at a state level
		// to that of the schema only if and when the key exists in the input.
		omitDefaults := make(schema.Defaults, len(providerSchema))
		for k := range providerSchema {
			omitDefaults[k] = schema.Omit
		}

		providerFieldMap = schema.FieldMap(providerSchema, omitDefaults)
	}

	coercedProviderCfg, err := providerFieldMap.Coerce(strConfig, nil)
	if err != nil {
		return nil, errors.Errorf(
			"coercing config for cloud type %q provider: %w",
			cloudType, err,
		)
	}

	providerCfg := coercedProviderCfg.(map[string]any)

	resultCfg := transform.Map(strConfig, func(k, v string) (string, any) { return k, v })

	jujuCfg, err := config.Coerce(strConfig)
	if err != nil {
		return nil, errors.Errorf("coercing config to Juju schema: %w", err)
	}

	for k, v := range providerCfg {
		resultCfg[k] = v
	}
	for k, v := range jujuCfg {
		resultCfg[k] = v
	}
	return resultCfg, nil
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
	return func(ctx context.Context) (_ modeldefaults.Defaults, err error) {
		ctx, span := trace.Start(ctx, trace.NameFromFunc())
		defer span.End()

		return s.ModelDefaults(ctx, uuid)
	}
}
