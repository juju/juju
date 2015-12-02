// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storageprovisioner

import (
	"github.com/juju/errors"
	"github.com/juju/names"
	"github.com/juju/utils/clock"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/storageprovisioner"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/dependency"
)

// ManifoldConfig defines a storage provisioner's configuration and dependencies.
type ManifoldConfig struct {
	APICallerName string
	ClockName     string

	Scope      names.Tag
	StorageDir string
}

// Manifold returns a dependency.Manifold that runs a storage provisioner.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{config.APICallerName, config.ClockName},
		Start: func(getResource dependency.GetResourceFunc) (worker.Worker, error) {

			var clock clock.Clock
			if err := getResource(config.ClockName, &clock); err != nil {
				return nil, errors.Trace(err)
			}
			var apiCaller base.APICaller
			if err := getResource(config.APICallerName, &apiCaller); err != nil {
				return nil, errors.Trace(err)
			}

			api, err := storageprovisioner.NewState(apiCaller, config.Scope)
			if err != nil {
				return nil, errors.Trace(err)
			}
			w, err := NewStorageProvisioner(Config{
				Scope:       config.Scope,
				StorageDir:  config.StorageDir,
				Volumes:     api,
				Filesystems: api,
				Life:        api,
				Environ:     api,
				Machines:    api,
				Status:      api,
				Clock:       clock,
			})
			if err != nil {
				return nil, errors.Trace(err)
			}
			return w, nil
		},
	}
}
