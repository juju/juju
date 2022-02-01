// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmdownloader

import (
	"github.com/juju/errors"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/dependency"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/base"
	client "github.com/juju/juju/api/charmdownloader"
)

// ManifoldConfig describes the resources used by the charmdownloader worker.
type ManifoldConfig struct {
	APICallerName string
	Logger        Logger

	// A constructor for the charmdownloader API which can be overridden
	// during testing. If omitted, the default client for the charmdownloader
	// facade will be automatically used.
	NewCharmDownloaderAPI func(base.APICaller) CharmDownloaderAPI
}

// Manifold returns a Manifold that encapsulates the charmdownloader worker.
func Manifold(cfg ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			cfg.APICallerName,
		},
		Start: cfg.start,
	}
}

// Validate is called by start to check for bad configuration.
func (cfg ManifoldConfig) Validate() error {
	if cfg.APICallerName == "" {
		return errors.NotValidf("empty APICallerName")
	}
	if cfg.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	return nil
}

// start is a StartFunc for a Worker manifold.
func (cfg ManifoldConfig) start(context dependency.Context) (worker.Worker, error) {
	if err := cfg.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	var apiConn api.Connection
	if err := context.Get(cfg.APICallerName, &apiConn); err != nil {
		return nil, errors.Trace(err)
	}

	newCharmDownloaderAPI := cfg.NewCharmDownloaderAPI
	if newCharmDownloaderAPI == nil {
		newCharmDownloaderAPI = func(apiCaller base.APICaller) CharmDownloaderAPI {
			return client.NewClient(apiCaller)
		}
	}

	w, err := NewCharmDownloader(Config{
		Logger:             cfg.Logger,
		CharmDownloaderAPI: newCharmDownloaderAPI(apiConn),
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return w, nil
}
