// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package objectstores3caller

import (
	context "context"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"

	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/internal/s3client"
	"github.com/juju/juju/internal/servicefactory"
)

// Logger represents the logging methods called.
type Logger interface {
	Errorf(message string, args ...any)
	Warningf(message string, args ...any)
	Infof(message string, args ...any)
	Debugf(message string, args ...any)
	Tracef(message string, args ...any)

	IsTraceEnabled() bool
}

// NewClientFunc is a function that returns a new S3 client.
type NewClientFunc func(string, s3client.HTTPClient, s3client.Credentials, s3client.Logger) (objectstore.Session, error)

// GetServiceFunc is a function that returns a service from the manifold.
type GetServiceFunc func(string, dependency.Getter, func(servicefactory.ControllerServiceFactory) ControllerService) (ControllerService, error)

// ManifoldConfig defines a Manifold's dependencies.
type ManifoldConfig struct {
	HTTPClientName     string
	ServiceFactoryName string

	// NewClient is used to create a new object store client.
	NewClient NewClientFunc
	// Logger is used to write logging statements for the worker.
	Logger Logger
	// Clock is used for the retry mechanism.
	Clock clock.Clock

	// GetService is used to get a service from the manifold.
	GetService GetServiceFunc
}

func (cfg ManifoldConfig) Validate() error {
	if cfg.HTTPClientName == "" {
		return errors.NotValidf("nil HTTPClientName")
	}
	if cfg.ServiceFactoryName == "" {
		return errors.NotValidf("nil ServiceFactoryName")
	}
	if cfg.NewClient == nil {
		return errors.NotValidf("nil NewClient")
	}
	if cfg.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	if cfg.Clock == nil {
		return errors.NotValidf("nil Clock")
	}
	return nil
}

// Manifold returns a manifold whose worker wraps an S3 Session.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.HTTPClientName,
			config.ServiceFactoryName,
		},
		Output: outputFunc,
		Start:  config.start,
	}
}

// start returns a StartFunc that creates a S3 client based on the supplied
// manifold config and wraps it in a worker.
func (config ManifoldConfig) start(ctx context.Context, getter dependency.Getter) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	controllerConfigService, err := config.GetService(config.ServiceFactoryName, getter, func(service servicefactory.ControllerServiceFactory) ControllerService {
		return service.ControllerConfig()
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	controllerConfig, err := controllerConfigService.ControllerConfig(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}

	// If we're not using S3, then we don't need to start this worker.
	if controllerConfig.ObjectStoreType() != objectstore.S3Backend {
		return nil, dependency.ErrUninstall
	}

	var httpClient s3client.HTTPClient
	if err := getter.Get(config.HTTPClientName, &httpClient); err != nil {
		return nil, errors.Trace(err)
	}

	return newS3Worker(workerConfig{
		ControllerService: controllerConfigService,
		HTTPClient:        httpClient,
		NewClient:         config.NewClient,
		Logger:            config.Logger,
		Clock:             config.Clock,
	})
}

// outputFunc extracts a S3 client from a *s3caller.
func outputFunc(in worker.Worker, out any) error {
	inWorker, _ := in.(*s3Worker)
	if inWorker == nil {
		return errors.Errorf("in should be a %T; got %T", inWorker, in)
	}

	switch outPointer := out.(type) {
	case *objectstore.Client:
		*outPointer = inWorker
	default:
		return errors.Errorf("out should be *s3caller.Session; got %T", out)
	}
	return nil
}

// NewS3Client returns a new S3 client based on the supplied dependencies.
func NewS3Client(url string, client s3client.HTTPClient, creds s3client.Credentials, logger s3client.Logger) (objectstore.Session, error) {
	return s3client.NewS3Client(url, client, creds, logger)
}

func GetService[A, B any](name string, getter dependency.Getter, fn func(A) B) (B, error) {
	var service A
	if err := getter.Get(name, &service); err != nil {
		var b B
		return b, errors.Trace(err)
	}

	return fn(service), nil
}
