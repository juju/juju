// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"

	jujucloud "github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
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
		if !errors.IsNotFound(err) {
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
