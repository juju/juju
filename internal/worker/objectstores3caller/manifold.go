// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package objectstores3caller

import (
	context "context"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"
	"gopkg.in/tomb.v2"

	coredependency "github.com/juju/juju/core/dependency"
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
type NewClientFunc func(endpoint string, client s3client.HTTPClient, creds s3client.Credentials, logger s3client.Logger) (objectstore.Session, error)

// GetControllerConfigServiceFunc is a helper function that gets a service from
// the manifold.
type GetControllerConfigServiceFunc func(getter dependency.Getter, name string) (ControllerConfigService, error)

// NewWorkerFunc is a function that returns a new worker.
type NewWorkerFunc func(workerConfig) (worker.Worker, error)

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

	// GetControllerConfigService is used to get a service from the manifold.
	GetControllerConfigService GetControllerConfigServiceFunc
	NewWorker                  NewWorkerFunc
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

	controllerConfigService, err := config.GetControllerConfigService(getter, config.ServiceFactoryName)
	if err != nil {
		return nil, errors.Trace(err)
	}
	controllerConfig, err := controllerConfigService.ControllerConfig(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}

	// If we're not using S3, then we don't need to start this worker.
	if controllerConfig.ObjectStoreType() != objectstore.S3Backend {
		return newNoopWorker(), nil
	}

	var httpClient s3client.HTTPClient
	if err := getter.Get(config.HTTPClientName, &httpClient); err != nil {
		return nil, errors.Trace(err)
	}

	return config.NewWorker(workerConfig{
		ControllerConfigService: controllerConfigService,
		HTTPClient:              httpClient,
		NewClient:               config.NewClient,
		Logger:                  config.Logger,
		Clock:                   config.Clock,
	})
}

// outputFunc extracts a S3 client from a *s3caller.
func outputFunc(in worker.Worker, out any) error {
	inWorker, err := outputWorker(in)
	if err != nil {
		return errors.Trace(err)
	}

	switch outPointer := out.(type) {
	case *objectstore.Client:
		*outPointer = inWorker
	default:
		return errors.Errorf("out should be *s3caller.Client; got %T", out)
	}
	return nil
}

func outputWorker(in worker.Worker) (objectstore.Client, error) {
	// First check if the worker is a s3Worker, otherwise, it's a noopWorker.
	// If neither case is true, then return an error.
	s3w, ok := in.(*s3Worker)
	if ok && s3w != nil {
		return s3w, nil
	}
	noopw, ok := in.(*noopWorker)
	if ok && noopw != nil {
		return noopw, nil
	}
	return nil, errors.Errorf("in should be *s3caller.Client; got %T", in)
}

// NewS3Client returns a new S3 client based on the supplied dependencies.
func NewS3Client(endpoint string, client s3client.HTTPClient, creds s3client.Credentials, logger s3client.Logger) (objectstore.Session, error) {
	return s3client.NewS3Client(endpoint, client, creds, logger)
}

// GetControllerConfigService is a helper function that gets a service from the
// manifold.
func GetControllerConfigService(getter dependency.Getter, name string) (ControllerConfigService, error) {
	return coredependency.GetDependencyByName(getter, name, func(factory servicefactory.ControllerServiceFactory) ControllerConfigService {
		return factory.ControllerConfig()
	})
}

type noopWorker struct {
	tomb tomb.Tomb
}

func newNoopWorker() worker.Worker {
	w := &noopWorker{}
	w.tomb.Go(func() error {
		<-w.tomb.Dying()
		return nil
	})
	return w
}

func (w *noopWorker) Kill() {
	w.tomb.Kill(nil)
}

func (w *noopWorker) Wait() error {
	return w.tomb.Wait()
}

func (w *noopWorker) Session(ctx context.Context, f func(context.Context, objectstore.Session) error) error {
	return errors.NotSupportedf("objectstore backend type is not set to s3: s3 caller")
}
