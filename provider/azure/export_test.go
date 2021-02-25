// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure

import (
	"github.com/juju/juju/environs"
	"github.com/juju/juju/storage"
)

func ForceVolumeSourceTokenRefresh(vs storage.VolumeSource) error {
	return ForceTokenRefresh(vs.(*azureVolumeSource).env)
}

func ForceTokenRefresh(env environs.Environ) error {
	return env.(*azureEnviron).authorizer.refresh()
}

func SetRetries(env environs.Environ) {
	azureEnv := env.(*azureEnviron)
	azureEnv.resources.RetryDuration = 0
	azureEnv.resources.RetryAttempts = 1
	azureEnv.compute.RetryDuration = 0
	azureEnv.compute.RetryAttempts = 1
	azureEnv.disk.RetryDuration = 0
	azureEnv.disk.RetryAttempts = 1
	azureEnv.network.RetryDuration = 0
	azureEnv.network.RetryAttempts = 1
}
