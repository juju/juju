// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasprober

import (
	"github.com/juju/errors"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/dependency"

	"github.com/juju/juju/apiserver/apiserverhttp"
)

// ManifoldConfig is the configuration used to setup a new caasprober.
type ManifoldConfig struct {
	// MuxName is the name of http api server used to register the probe
	// endpoints for this worker.
	MuxName string
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

func (c ManifoldConfig) Start(context dependency.Context) (worker.Worker, error) {
	if err := c.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	var mux *apiserverhttp.Mux
	if err := context.Get(c.MuxName, &mux); err != nil {
		return nil, errors.Trace(err)
	}

	return NewController(NewCAASProbes(), mux)
}

func (c ManifoldConfig) Validate() error {
	if c.MuxName == "" {
		return errors.NotValidf("empty mux name")
	}
	return nil
}
