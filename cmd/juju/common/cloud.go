// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
)

var logger = loggo.GetLogger("juju.cmd.juju.common")

// CloudOrProvider finds and returns cloud or provider.
func CloudOrProvider(cloudName string, cloudByNameFunc func(string) (*cloud.Cloud, error)) (cloud *cloud.Cloud, err error) {
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

// BuiltInClouds returns cloud information for those
// providers which are built in to Juju.
func BuiltInClouds() (map[string]cloud.Cloud, error) {
	allClouds := make(map[string]cloud.Cloud)
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
