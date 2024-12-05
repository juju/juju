// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasprobebinder

import (
	"context"
	"maps"

	"github.com/juju/errors"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"

	"github.com/juju/juju/internal/observability/probe"
	"github.com/juju/juju/internal/worker/caasprober"
)

// ManifoldConfig is the configuration used to setup a new caasprober.
type ManifoldConfig struct {
	// ProberName is the name of the caasprober worker to get from the
	// dependency engine.
	ProberName string

	// ProbeProviderNames is a list of probe providers to fetch from the
	// dependency engine.
	ProbeProviderNames []string

	// DefaultProviders is a list of probe providers that are given to this
	// worker at instantiation and not fetched from the dependency engine.
	DefaultProviders map[string]probe.ProbeProvider
}

// Manifold returns a new manifold for a caasprobebinder worker.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: append([]string{config.ProberName}, config.ProbeProviderNames...),
		Start:  config.Start,
	}
}

func (c ManifoldConfig) Start(ctx context.Context, getter dependency.Getter) (worker.Worker, error) {
	if err := c.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	var probes *caasprober.CAASProbes
	if err := getter.Get(c.ProberName, &probes); err != nil {
		return nil, errors.Trace(err)
	}

	providers := maps.Clone(c.DefaultProviders)
	if providers == nil {
		providers = make(map[string]probe.ProbeProvider)
	}
	for _, k := range c.ProbeProviderNames {
		var provider probe.ProbeProvider
		if err := getter.Get(k, &provider); err != nil {
			return nil, errors.Trace(err)
		}
		providers[k] = provider
	}

	return NewProbeBinder(probes, providers)
}

func (c ManifoldConfig) Validate() error {
	if c.ProberName == "" {
		return errors.NotValidf("empty prober name")
	}
	return nil
}
