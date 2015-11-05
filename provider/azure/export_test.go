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
	return env.(*azureEnviron).config.token.Refresh()
}
