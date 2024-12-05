// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasprober

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"

	"github.com/juju/juju/apiserver/apiserverhttp"
	"github.com/juju/juju/internal/observability/probe"
)

// ManifoldConfig is the configuration used to setup a new caasprober.
type ManifoldConfig struct {
	// MuxName is the name of http api server used to register the probe
	// endpoints for this worker.
	MuxName string

	// DefaultProviders is a list of probe providers that are given to this
	// worker at instantiation and not fetched from the dependency engine.
	DefaultProviders map[string]probe.ProbeProvider

	// Providers is a list of worker providers that can offer one of the Prober
	// interfaces to be registered in this worker. Expects at least one of
	// LivenessProber, ReadinessProber or StartupProber.
	Providers []string
}

// gatherCAASProbes is responsible for taking all the probe dependencies
// passed into the manifold and producing a set of CAASProbes that can be run
// as part of this worker.
func gatherCAASProbes(
	getter dependency.Getter,
	defaultProviders map[string]probe.ProbeProvider,
	providers []string) (*CAASProbes, error,
) {
	probes := NewCAASProbes()

	// General add function that can be called for the 2 different types of
	// providers we receive.
	addProvider := func(id string, provider probe.ProbeProvider) {
		supported := provider.SupportedProbes()

		if supported.Supports(probe.ProbeLiveness) {
			probes.probes[probe.ProbeLiveness].AddProber(id, supported[probe.ProbeLiveness])
		}

		if supported.Supports(probe.ProbeReadiness) {
			probes.probes[probe.ProbeReadiness].AddProber(id, supported[probe.ProbeReadiness])
		}

		if supported.Supports(probe.ProbeStartup) {
			probes.probes[probe.ProbeStartup].AddProber(id, supported[probe.ProbeStartup])
		}
	}

	if providers == nil {
		providers = []string{}
	}
	for _, provider := range providers {
		var probeProvider probe.ProbeProvider
		if err := getter.Get(provider, &probeProvider); err != nil {
			return probes, errors.Trace(err)
		}

		addProvider(provider, probeProvider)
	}

	if defaultProviders == nil {
		defaultProviders = map[string]probe.ProbeProvider{}
	}
	for k, provider := range defaultProviders {
		addProvider(k, provider)
	}

	return probes, nil
}

func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{config.MuxName},
		Start:  config.Start,
		Output: func(in worker.Worker, out interface{}) error {
			controller, _ := in.(*Controller)
			if controller == nil {
				return errors.Errorf("expected Controller in")
			}
			switch outPtr := out.(type) {
			case **CAASProbes:
				*outPtr = controller.probes
			default:
				return errors.Errorf("unknown out type")
			}
			return nil
		},
	}
}

func (c ManifoldConfig) Start(context context.Context, getter dependency.Getter) (worker.Worker, error) {
	if err := c.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	var mux *apiserverhttp.Mux
	if err := getter.Get(c.MuxName, &mux); err != nil {
		return nil, errors.Trace(err)
	}

	probes, err := gatherCAASProbes(getter, c.DefaultProviders, c.Providers)
	if err != nil {
		return nil, errors.Trace(err)
	}

	return NewController(probes, mux)
}

func (c ManifoldConfig) Validate() error {
	if c.MuxName == "" {
		return errors.NotValidf("empty mux name")
	}
	return nil
}
