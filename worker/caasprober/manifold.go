// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasprober

import (
	"github.com/juju/errors"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/dependency"

	"github.com/juju/juju/v2/apiserver/apiserverhttp"
	"github.com/juju/juju/v2/observability/probe"
)

// ManifoldConfig is the configuration used to setup a new caasprober.
type ManifoldConfig struct {
	// MuxName is the name of http api server used to register the probe
	// endpoints for this worker.
	MuxName string

	// Providers is a list of worker providers that can offer one of the Prober
	// interfaces to be registered in this worker. Expects at least one of
	// LivenessProber, ReadinessProber or StartupProber.
	Providers []string
}

func gatherCAASProbes(providers []string, context dependency.Context) (*CAASProbes, error) {
	probes := NewCAASProbes()

	for _, provider := range providers {
		var probeProvider probe.ProbeProvider
		if err := context.Get(provider, &probeProvider); err != nil {
			return probes, errors.Trace(err)
		}
		supported := probeProvider.SupportedProbes()

		if supported.Supports(probe.ProbeLiveness) {
			probes.Liveness.Probes[provider] = supported[probe.ProbeLiveness]
		}

		if supported.Supports(probe.ProbeReadiness) {
			probes.Readiness.Probes[provider] = supported[probe.ProbeReadiness]
		}

		if supported.Supports(probe.ProbeStartup) {
			probes.Readiness.Probes[provider] = supported[probe.ProbeStartup]
		}
	}

	return probes, nil
}

func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: append([]string{config.MuxName}, config.Providers...),
		Output: nil,
		Start:  config.Start,
	}
}

func (c ManifoldConfig) Start(context dependency.Context) (worker.Worker, error) {
	if err := c.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	var mux *apiserverhttp.Mux
	if err := context.Get(c.MuxName, &mux); err != nil {
		return nil, errors.Trace(err)
	}

	probes, err := gatherCAASProbes(c.Providers, context)
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
