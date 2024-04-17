// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"context"
	"fmt"

	"github.com/juju/errors"

	coremodel "github.com/juju/juju/core/model"
	modelerrors "github.com/juju/juju/domain/model/errors"
	"github.com/juju/juju/environs/config"
)

// State is a testing in memory implementation of State for model defaults
// service.
type State struct {
	// CloudDefaults represents the cloud defaults on a per model basis and is
	// what is returned.
	CloudDefaults map[coremodel.UUID]map[string]string

	// CloudRegionDefaults represents the defaults set for a models cloud region
	// and is what is returned for ModelCloudRegionDefaults.
	CloudRegionDefaults map[coremodel.UUID]map[string]string

	// ProviderConfigSchema represents the provider defaults returned for a given
	// model and is what is returned in ModelProviderconfigSchema.
	ProviderConfigSchema map[coremodel.UUID]config.ConfigSchemaSource

	// Defaults is the values returned for ConfigDefaults. If this value is nil
	// then the defaults recorded in environs config is returned.
	Defaults map[string]any

	// MetadataDefaults  maintains a list of metadata defaults for each model
	// uuid.
	MetadataDefaults map[coremodel.UUID]map[string]string
}

// ConfigDefaults returns the default configuration values set in Juju.
func (s *State) ConfigDefaults(_ context.Context) map[string]any {
	if s.Defaults == nil {
		return config.ConfigDefaults()
	}
	return s.Defaults
}

// ModelCloudDefaults returns the defaults associated with the model's cloud.
func (s *State) ModelCloudDefaults(_ context.Context, uuid coremodel.UUID) (map[string]string, error) {
	defaults, exists := s.CloudDefaults[uuid]
	if !exists {
		return map[string]string{}, fmt.Errorf("%w %q", modelerrors.NotFound, uuid)
	}
	return defaults, nil
}

// ModelCloudRegionDefaults returns the defaults associated with the models
// set cloud region.
func (s *State) ModelCloudRegionDefaults(_ context.Context, uuid coremodel.UUID) (map[string]string, error) {
	if defaults, exists := s.CloudRegionDefaults[uuid]; exists {
		return defaults, nil
	}
	return map[string]string{}, nil
}

// ModelProviderConfigSchema returns the providers config schema source based on
// the cloud set for the model.
func (s *State) ModelProviderConfigSchema(_ context.Context, uuid coremodel.UUID) (config.ConfigSchemaSource, error) {
	if schemaSource, exists := s.ProviderConfigSchema[uuid]; exists {
		return schemaSource, nil
	}
	return nil, errors.NotFound
}

// ModelMetadataDefaults returns the default values for the model specified by
// uuid.
func (s *State) ModelMetadataDefaults(_ context.Context, uuid coremodel.UUID) (map[string]string, error) {
	defaults, exists := s.MetadataDefaults[uuid]
	if !exists {
		return map[string]string{}, fmt.Errorf("%w %q", modelerrors.NotFound, uuid)
	}
	return defaults, nil
}
