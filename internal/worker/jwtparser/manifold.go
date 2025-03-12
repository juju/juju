// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jwtparser

import (
	"context"
	"net/http"
	"time"

	"github.com/juju/errors"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"

	coredependency "github.com/juju/juju/core/dependency"
	"github.com/juju/juju/internal/jwtparser"
	"github.com/juju/juju/internal/services"
)

// GetControllerConfigServiceFunc is a helper function that gets
// a controller config service from the manifold.
type GetControllerConfigServiceFunc = func(getter dependency.Getter, name string) (ControllerConfigService, error)

// GetControllerConfigService is a helper function that gets a service from the
// manifold.
func GetControllerConfigService(getter dependency.Getter, name string) (ControllerConfigService, error) {
	return coredependency.GetDependencyByName(getter, name, func(factory services.ControllerDomainServices) ControllerConfigService {
		return factory.ControllerConfig()
	})
}

// ManifoldConfig defines a Manifold's dependencies.
type ManifoldConfig struct {
	// DomainServicesName is the name of the domain services worker.
	DomainServicesName string
	// GetControllerConfigService is used to get a service from the manifold.
	GetControllerConfigService GetControllerConfigServiceFunc
}

// Manifold returns a manifold whose worker wraps a JWT parser.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.DomainServicesName,
		},
		Output: outputFunc,
		Start:  config.start,
	}
}

func (config ManifoldConfig) start(_ context.Context, getter dependency.Getter) (worker.Worker, error) {
	controllerConfigService, err := config.GetControllerConfigService(getter, config.DomainServicesName)
	if err != nil {
		return nil, errors.Trace(err)
	}

	return NewWorker(controllerConfigService, defaultHTTPClient())
}

// outputFunc extracts a jwtparser.Parser from a
// jwtParserWorker contained within a CleanupWorker.
func outputFunc(in worker.Worker, out interface{}) error {
	inWorker, _ := in.(*jwtParserWorker)
	if inWorker == nil {
		return errors.Errorf("in should be a %T; got %T", inWorker, in)
	}

	switch outPointer := out.(type) {
	case **jwtparser.Parser:
		*outPointer = inWorker.jwtParser
	default:
		return errors.Errorf("out should be jwtparser.Parser; got %T", out)
	}
	return nil
}

// defaultHTTPClient returns a defaulthttp client
// that follows redirects with a sensible timeout.
func defaultHTTPClient() HTTPClient {
	return &http.Client{
		Timeout: 30 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}
