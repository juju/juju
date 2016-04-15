// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
)

var logger = loggo.GetLogger("juju.cmd.juju.cloud")

// CloudOrProvider finds and returns cloud or provider.
func CloudOrProvider(cloudName string, cloudByNameFunc func(string) (*cloud.Cloud, error)) (cloud *cloud.Cloud, err error) {
	if cloud, err = cloudByNameFunc(cloudName); err != nil {
		if !errors.IsNotFound(err) {
			return nil, err
		}
		builtInProviders := BuiltInProviders()
		if builtIn, ok := builtInProviders[cloudName]; !ok {
			return nil, errors.NotValidf("cloud %v", cloudName)
		} else {
			cloud = &builtIn
		}
	}
	return cloud, nil
}

// BuiltInProviders returns cloud information for those
// providers which are built in to Juju.
func BuiltInProviders() map[string]cloud.Cloud {
	builtIn := make(map[string]cloud.Cloud)
	for _, name := range cloud.BuiltInProviderNames {
		provider, err := environs.Provider(name)
		if err != nil {
			// Should never happen but it will on go 1.2
			// because lxd provider is not built.
			logger.Warningf("cloud %q not available on this platform", name)
			continue
		}
		var regions []cloud.Region
		if detector, ok := provider.(environs.CloudRegionDetector); ok {
			regions, err = detector.DetectRegions()
			if err != nil && !errors.IsNotFound(err) {
				logger.Warningf("could not detect regions for %q: %v", name, err)
			}
		}
		aCloud := cloud.Cloud{
			Type:    name,
			Regions: regions,
		}
		schema := provider.CredentialSchemas()
		for authType := range schema {
			if authType == cloud.EmptyAuthType {
				continue
			}
			aCloud.AuthTypes = append(aCloud.AuthTypes, authType)
		}
		builtIn[name] = aCloud
	}
	return builtIn
}
