// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/schema"

	"github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/config"
)

type Service struct {
	cloudDefaultsState CloudDefaultsState
	cloudService       CloudService
	staticProvider     StaticConfigProvider
}

func NewService(
	cloudDefaultsState CloudDefaultsState,
	cloudService CloudService,
	staticProvider StaticConfigProvider) *Service {
	return &Service{
		cloudDefaultsState: cloudDefaultsState,
		cloudService:       cloudService,
		staticProvider:     staticProvider,
	}
}

// ModelDefaults is responsible for returning the default configuration set
// for a specific cloud. It combines all the defaults from the provider, cloud
// and cloud regions.
func (s *Service) ModelDefaults(ctx context.Context, name string) (config.ModelDefaultAttributes, error) {
	defaults := make(config.ModelDefaultAttributes)
	for k, v := range s.staticProvider.ConfigDefaults() {
		defaults[k] = config.AttributeDefaultValues{Default: v}
	}

	cloud, err := s.cloudService.Get(ctx, name)
	if err != nil {
		return defaults, errors.Trace(err)
	}

	cloudSchemaSource, err := s.staticProvider.CloudConfig(cloud.Type)
	if err != nil && !errors.Is(err, errors.NotFound) {
		return defaults, errors.Trace(err)
	} else if !errors.Is(err, errors.NotFound) {
		fields := schema.FieldMap(cloudSchemaSource.ConfigSchema(), cloudSchemaSource.ConfigDefaults())
		coercedAttrs, err := fields.Coerce(defaults, nil)
		if err != nil {
			return defaults, errors.Trace(err)
		}

		for k, v := range coercedAttrs.(map[string]interface{}) {
			defaults[k] = config.AttributeDefaultValues{Default: v}
		}
	}

	cloudDefaults, err := s.cloudDefaultsState.CloudDefaults(ctx, name)
	if err != nil {
		return defaults, fmt.Errorf("getting cloud %q defaults: %w", name, err)
	}

	for k, v := range cloudDefaults {
		ds := defaults[k]
		ds.Controller = v
		defaults[k] = ds
	}

	allRegionDefaults, err := s.cloudDefaultsState.CloudAllRegionDefaults(ctx, name)
	if err != nil {
		return defaults, fmt.Errorf("get cloud %q region defaults: %w", name, err)
	}

	for region, regionDefaults := range allRegionDefaults {
		for k, v := range regionDefaults {
			regCfg := config.RegionDefaultValue{Name: region, Value: v}
			ds := defaults[k]
			if ds.Regions == nil {
				ds.Regions = make([]config.RegionDefaultValue, 0, 1)
			}
			ds.Regions = append(ds.Regions, regCfg)
			defaults[k] = ds
		}
	}

	return defaults, nil
}

// UpdateModelDefaults will update the defaults values for either a cloud or a
// cloud region. Not specifying a valid value for either Cloud or Region will
// result in an error that satisfies NotValid.
func (s *Service) UpdateModelDefaults(
	ctx context.Context,
	updateAttrs map[string]string,
	removeAttrs []string,
	spec cloudspec.CloudRegionSpec,
) error {
	if spec.Cloud == "" && spec.Region == "" {
		return fmt.Errorf("%w either cloud or cloud and region need to be supplied", errors.NotValid)
	}

	if spec.Region == "" {
		return errors.Trace(s.cloudDefaultsState.UpdateCloudDefaults(ctx, spec.Cloud, updateAttrs, removeAttrs))
	} else {
		return errors.Trace(s.cloudDefaultsState.UpdateCloudRegionDefaults(ctx, spec.Cloud, spec.Region, updateAttrs, removeAttrs))
	}
}
