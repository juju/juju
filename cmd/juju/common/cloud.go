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
		builtInClouds := BuiltInClouds()
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
func BuiltInClouds() map[string]cloud.Cloud {
	// TODO (anastasiamac 2016-04-14)
	// This whole method will be redundant after we move to 1.3+.
	builtIn := make(map[string]cloud.Cloud)
	for name, aCloud := range cloud.BuiltInClouds {
		_, err := environs.Provider(aCloud.Type)
		if err != nil {
			// Should never happen but it will on go 1.2
			// because lxd provider is not built.
			logger.Warningf("cloud %q not available on this platform", name)
			continue
		}
		builtIn[name] = aCloud
	}
	return builtIn
}
