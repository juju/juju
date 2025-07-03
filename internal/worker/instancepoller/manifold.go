// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instancepoller

import (
	"context"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"

	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/internal/services"
)

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
	DomainServicesName string
	Clock              clock.Clock
	EnvironName        string
	Logger             logger.Logger
}

func (config ManifoldConfig) start(context context.Context, getter dependency.Getter) (worker.Worker, error) {
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

	var domainServices services.ModelDomainServices
	if err := getter.Get(config.DomainServicesName, &domainServices); err != nil {
		return nil, errors.Trace(err)
	}

	w, err := NewWorker(Config{
		Clock:          config.Clock,
		MachineService: domainServices.Machine(),
		StatusService:  domainServices.Status(),
		NetworkService: domainServices.Network(),
		Environ:        netEnv,
		Logger:         config.Logger,
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
			config.DomainServicesName,
			config.EnvironName,
		},
		Start: config.start,
	}
}
