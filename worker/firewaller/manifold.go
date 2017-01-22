// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package firewaller

import (
	"github.com/juju/errors"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/firewaller"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/dependency"
)

// ManifoldConfig describes the resources used by the firewaller worker.
type ManifoldConfig struct {
	APICallerName string
	EnvironName   string
}

// Manifold returns a Manifold that encapsulates the firewaller worker.
func Manifold(cfg ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			cfg.APICallerName,
			cfg.EnvironName,
		},
		Start: func(context dependency.Context) (worker.Worker, error) {
			var apiCaller base.APICaller
			if err := context.Get(cfg.APICallerName, &apiCaller); err != nil {
				return nil, errors.Trace(err)
			}
			var environ environs.Environ
			if err := context.Get(cfg.EnvironName, &environ); err != nil {
				return nil, errors.Trace(err)
			}
			mode := environ.Config().FirewallMode()
			if mode == config.FwNone {
				logger.Infof("stopping firewaller (not required)")
				return nil, dependency.ErrUninstall
			}
			return manifoldStart(environ, apiCaller, mode)
		},
	}
}

// manifoldStart creates a firewaller worker, given a base.APICaller.
func manifoldStart(env environs.Environ, apiCaller base.APICaller, firewallMode string) (worker.Worker, error) {
	api := firewaller.NewState(apiCaller)
	w, err := NewFirewaller(env, api, firewallMode)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return w, nil
}
