// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"bytes"

	"github.com/juju/cmd/v3"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"gopkg.in/juju/environschema.v1"

	jujucloud "github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
)

var logger = loggo.GetLogger("juju.cmd.juju.common")

type chooseCloudRegionError struct {
	error
}

// IsChooseCloudRegionError reports whether or not the given
// error was returned from ChooseCloudRegion.
func IsChooseCloudRegionError(err error) bool {
	_, ok := errors.Cause(err).(chooseCloudRegionError)
	return ok
}

// CloudOrProvider finds and returns cloud or provider.
func CloudOrProvider(cloudName string, cloudByNameFunc func(string) (*jujucloud.Cloud, error)) (cloud *jujucloud.Cloud, err error) {
	if cloud, err = cloudByNameFunc(cloudName); err != nil {
		if !errors.Is(err, errors.NotFound) {
			return nil, err
		}
		builtInClouds, err := BuiltInClouds()
		if err != nil {
			return nil, errors.Trace(err)
		}
		if builtIn, ok := builtInClouds[cloudName]; !ok {
			return nil, errors.NotValidf("cloud %v", cloudName)
		} else {
			cloud = &builtIn
		}
	}
	return cloud, nil
}

// ChooseCloudRegion returns the cloud.Region to use, based on the specified
// region name. If no region name is specified, and there is at least one
// region, we use the first region in the list. If there are no regions, then
// we return a region with no name, having the same endpoints as the cloud.
func ChooseCloudRegion(cloud jujucloud.Cloud, regionName string) (jujucloud.Region, error) {
	if regionName != "" {
		region, err := jujucloud.RegionByName(cloud.Regions, regionName)
		if err != nil {
			return jujucloud.Region{}, errors.Trace(chooseCloudRegionError{err})
		}
		return *region, nil
	}
	if len(cloud.Regions) > 0 {
		// No region was specified, use the first region in the list.
		return cloud.Regions[0], nil
	}
	return jujucloud.Region{
		"", // no region name
		cloud.Endpoint,
		cloud.IdentityEndpoint,
		cloud.StorageEndpoint,
	}, nil
}

// BuiltInClouds returns cloud information for those
// providers which are built in to Juju.
func BuiltInClouds() (map[string]jujucloud.Cloud, error) {
	allClouds := make(map[string]jujucloud.Cloud)
	for _, providerType := range environs.RegisteredProviders() {
		p, err := environs.Provider(providerType)
		if err != nil {
			return nil, errors.Trace(err)
		}
		detector, ok := p.(environs.CloudDetector)
		if !ok {
			continue
		}
		clouds, err := detector.DetectClouds()
		if err != nil {
			return nil, errors.Annotatef(
				err, "detecting clouds for provider %q",
				providerType,
			)
		}
		for _, cloud := range clouds {
			allClouds[cloud.Name] = cloud
		}
	}
	return allClouds, nil
}

// CloudByName returns a cloud for given name
// regardless of whether it's public, private or builtin cloud.
// Not to be confused with cloud.CloudByName which does not cater
// for built-in clouds like localhost.
func CloudByName(cloudName string) (*jujucloud.Cloud, error) {
	cloud, err := jujucloud.CloudByName(cloudName)
	if err != nil {
		if errors.Is(err, errors.NotFound) {
			// Check built in clouds like localhost (lxd).
			builtinClouds, err := BuiltInClouds()
			if err != nil {
				return nil, errors.Trace(err)
			}
			aCloud, found := builtinClouds[cloudName]
			if !found {
				return nil, errors.NotFoundf("cloud %s", cloudName)
			}
			return &aCloud, nil
		}
		return nil, errors.Trace(err)
	}
	return cloud, nil
}

// CloudSchemaByType returns the Schema for a given cloud type.
// If the ProviderSchema is not implemented for the given cloud
// type, a NotFound error is returned.
func CloudSchemaByType(cloudType string) (environschema.Fields, error) {
	provider, err := environs.Provider(cloudType)
	if err != nil {
		return nil, err
	}
	ps, ok := provider.(environs.ProviderSchema)
	if !ok {
		return nil, errors.NotImplementedf("environs.ProviderSchema")
	}
	providerSchema := ps.Schema()
	if providerSchema == nil {
		return nil, errors.New("Failed to retrieve Provider Schema")
	}
	return providerSchema, nil
}

// ProviderConfigSchemaSourceByType returns a config.ConfigSchemaSource
// for the environ provider, found for the given cloud type, or an error.
func ProviderConfigSchemaSourceByType(cloudType string) (config.ConfigSchemaSource, error) {
	provider, err := environs.Provider(cloudType)
	if err != nil {
		return nil, err
	}
	if cs, ok := provider.(config.ConfigSchemaSource); ok {
		return cs, nil
	}
	return nil, errors.NotImplementedf("config.ConfigSource")
}

// PrintConfigSchema is used to print model configuration schema.
type PrintConfigSchema struct {
	Type        string `yaml:"type,omitempty" json:"type,omitempty"`
	Description string `yaml:"description,omitempty" json:"description,omitempty"`
}

func FormatConfigSchema(values interface{}) (string, error) {
	out := &bytes.Buffer{}
	err := cmd.FormatSmart(out, values)
	if err != nil {
		return "", err
	}
	return out.String(), nil
}
