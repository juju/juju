// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instancepoller

import (
	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/juju/worker/v2"
	"github.com/juju/worker/v2/dependency"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/instancepoller"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/worker/common"
)

// Logger represents the methods used by the worker to log details.
type Logger interface {
	Tracef(string, ...interface{})
	Debugf(string, ...interface{})
	Infof(string, ...interface{})
	Warningf(string, ...interface{})
	Errorf(string, ...interface{})
}

// facadeShim wraps an instancepoller API instance and allows us to provide
// methods that return interfaces which we can easily mock in our tests.
type facadeShim struct {
	api *instancepoller.API
}

func (s facadeShim) Machine(tag names.MachineTag) (Machine, error) { return s.api.Machine(tag) }
func (s facadeShim) WatchModelMachines() (watcher.StringsWatcher, error) {
	return s.api.WatchModelMachines()
}

var errNetworkingNotSupported = errors.NotSupportedf("networking")

// environWithoutNetworking wraps a environs.Environ instance that does not
// support environs.Networking so that calls to NetworkInterfaces always
// return a NotSupported error.
type environWithoutNetworking struct {
	env environs.Environ
}

func (e environWithoutNetworking) Instances(ctx context.ProviderCallContext, ids []instance.Id) ([]instances.Instance, error) {
	return e.env.Instances(ctx, ids)
}

func (e environWithoutNetworking) NetworkInterfaces(context.ProviderCallContext, []instance.Id) ([][]network.InterfaceInfo, error) {
	return nil, errNetworkingNotSupported
}

// ManifoldConfig describes the resources used by the instancepoller worker.
type ManifoldConfig struct {
	APICallerName string
	ClockName     string
	EnvironName   string
	Logger        Logger

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

	// If the current environment does not support networking use a shim
	// whose NetworkInterfaces method always returns a NotSupported error.
	netEnv, supported := environ.(Environ)
	if !supported {
		netEnv = &environWithoutNetworking{env: environ}
	}

	var apiCaller base.APICaller
	if err := context.Get(config.APICallerName, &apiCaller); err != nil {
		return nil, errors.Trace(err)
	}

	credentialAPI, err := config.NewCredentialValidatorFacade(apiCaller)
	if err != nil {
		return nil, errors.Trace(err)
	}

	w, err := NewWorker(Config{
		Clock: clock,
		Facade: facadeShim{
			api: instancepoller.NewAPI(apiCaller),
		},
		Environ:       netEnv,
		Logger:        config.Logger,
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
