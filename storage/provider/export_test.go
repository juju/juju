// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import "github.com/juju/juju/storage"

var CommonProviders = commonProviders

func LoopVolumeSource(storageDir string, run func(string, ...string) (string, error)) storage.VolumeSource {
	return &loopVolumeSource{run, storageDir}
}

func LoopProvider(run func(string, ...string) (string, error)) storage.Provider {
	return &loopProvider{run}
}
