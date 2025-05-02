// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instancepoller

import (
	"context"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/controller/instancepoller"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/instances"
)

// facadeShim wraps an instancepoller API instance and allows us to provide
// methods that return interfaces which we can easily mock in our tests.
type facadeShim struct {
	api *instancepoller.API
}

func (s facadeShim) Machine(ctx context.Context, tag names.MachineTag) (Machine, error) {
	return s.api.Machine(ctx, tag)
}
func (s facadeShim) WatchModelMachines(ctx context.Context) (watcher.StringsWatcher, error) {
	return s.api.WatchModelMachines(ctx)
}

var errNetworkingNotSupported = errors.NotSupportedf("networking")

// environWithoutNetworking wraps a environs.Environ instance that does not
// support environs.Networking so that calls to NetworkInterfaces always
// return a NotSupported error.
type environWithoutNetworking struct {
	env environs.Environ
}

func (e environWithoutNetworking) Instances(ctx context.Context, ids []instance.Id) ([]instances.Instance, error) {
	return e.env.Instances(ctx, ids)
}

func (e environWithoutNetworking) NetworkInterfaces(context.Context, []instance.Id) ([]network.InterfaceInfos, error) {
	return nil, errNetworkingNotSupported
}

// ManifoldConfig describes the resources used by the instancepoller worker.
type ManifoldConfig struct {
	APICallerName string
	ClockName     string
	EnvironName   string
	Logger        logger.Logger
}

func (config ManifoldConfig) start(context context.Context, getter dependency.Getter) (worker.Worker, error) {
	var clock clock.Clock
	if err := getter.Get(config.ClockName, &clock); err != nil {
		return nil, errors.Trace(err)
	}
	var environ environs.Environ
	if err := getter.Get(config.EnvironName, &environ); err != nil {
		return nil, errors.Trace(err)
	}

	// If the current environment does not support networking use a shim
	// whose NetworkInterfaces method always returns a NotSupported error.
	netEnv, supported := environ.(Environ)
	if !supported {
		netEnv = &environWithoutNetworking{env: environ}
	}

	var apiCaller base.APICaller
	if err := getter.Get(config.APICallerName, &apiCaller); err != nil {
		return nil, errors.Trace(err)
	}

	w, err := NewWorker(Config{
		Clock: clock,
		Facade: facadeShim{
			api: instancepoller.NewAPI(apiCaller),
		},
		Environ: netEnv,
		Logger:  config.Logger,
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
