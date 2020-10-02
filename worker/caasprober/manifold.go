// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasprober

import (
	"github.com/juju/errors"
	"github.com/juju/worker/v2"
	"github.com/juju/worker/v2/dependency"

	"github.com/juju/juju/apiserver/apiserverhttp"
)

type ManifoldConfig struct {
	MuxName string
}

func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.MuxName,
		},
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

	return NewController(&caasProbes{
		Liveness:  &ProbeSuccess{},
		Readiness: &ProbeSuccess{},
		Startup:   &ProbeSuccess{},
	}, mux)
}

func (c ManifoldConfig) Validate() error {
	if c.MuxName == "" {
		return errors.NotValidf("empty mux name")
	}
	return nil
}
