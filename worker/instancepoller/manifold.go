// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instancepoller

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/utils/clock"
	worker "gopkg.in/juju/worker.v1"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/instancepoller"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/worker/common"
	"github.com/juju/juju/worker/dependency"
)

// ManifoldConfig describes the resources used by the instancepoller worker.
type ManifoldConfig struct {
	APICallerName string
	ClockName     string
	Delay         time.Duration
	EnvironName   string

	NewCredentialValidatorFacade func(base.APICaller) (common.CredentialAPI, error)
}

func (config ManifoldConfig) start(context dependency.Context) (worker.Worker, error) {
	var clock clock.Clock
	if err := context.Get(config.ClockName, &clock); err != nil {
		return nil, errors.Trace(err)
	}
	var environ environs.Environ
	if err := context.Get(config.EnvironName, &environ); err != nil {
		return nil, errors.Trace(err)
	}

	var apiCaller base.APICaller
	if err := context.Get(config.APICallerName, &apiCaller); err != nil {
		return nil, errors.Trace(err)
	}
	facade := instancepoller.NewAPI(apiCaller)

	credentialAPI, err := config.NewCredentialValidatorFacade(apiCaller)
	if err != nil {
		return nil, errors.Trace(err)
	}

	w, err := NewWorker(Config{
		Clock:         clock,
		Delay:         config.Delay,
		Facade:        facade,
		Environ:       environ,
		CredentialAPI: credentialAPI,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return w, nil
}

// Manifold returns a Manifold that encapsulates the instancepoller worker.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.APICallerName,
			config.EnvironName,
			config.ClockName,
		},
		Start: config.start,
	}
}
