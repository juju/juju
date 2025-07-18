// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package objectstores3caller

import (
	"context"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"

	coredependency "github.com/juju/juju/core/dependency"
	corehttp "github.com/juju/juju/core/http"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/internal/s3client"
	"github.com/juju/juju/internal/services"
)

// NewClientFunc is a function that returns a new S3 client.
type NewClientFunc = func(endpoint string, client s3client.HTTPClient, creds s3client.Credentials, logger logger.Logger) (objectstore.Session, error)

// GetControllerConfigServiceFunc is a helper function that gets a service from
// the manifold.
type GetControllerConfigServiceFunc = func(getter dependency.Getter, name string) (ControllerConfigService, error)

// GetGuardServiceFunc is a function that retrieves the
// controller object store services from the dependency getter.
type GetGuardServiceFunc func(dependency.Getter, string) (GuardService, error)

// GuardService provides access to the object store for draining
// operations.
type GuardService interface {
	// GetDrainingPhase returns the current active draining phase of the
	// object store.
	GetDrainingPhase(ctx context.Context) (objectstore.Phase, error)

	// WatchDraining returns a watcher that watches the draining phase of the
	// object store.
	WatchDraining(ctx context.Context) (watcher.Watcher[struct{}], error)
}

// NewWorkerFunc is a function that returns a new worker.
type NewWorkerFunc = func(workerConfig) (worker.Worker, error)

// ManifoldConfig defines a Manifold's dependencies.
type ManifoldConfig struct {
	HTTPClientName          string
	ObjectStoreServicesName string

	// NewClient is used to create a new object store client.
	NewClient NewClientFunc
	// Logger is used to write logging statements for the worker.
	Logger logger.Logger
	// Clock is used for the retry mechanism.
	Clock clock.Clock

	// GetControllerConfigService is used to get a service from the manifold.
	GetControllerConfigService GetControllerConfigServiceFunc
	GetGuardService            GetGuardServiceFunc
	NewWorker                  NewWorkerFunc
}

func (cfg ManifoldConfig) Validate() error {
	if cfg.HTTPClientName == "" {
		return errors.NotValidf("nil HTTPClientName")
	}
	if cfg.ObjectStoreServicesName == "" {
		return errors.NotValidf("nil ObjectStoreServicesName")
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
			config.ObjectStoreServicesName,
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

	controllerConfigService, err := config.GetControllerConfigService(getter, config.ObjectStoreServicesName)
	if err != nil {
		return nil, errors.Trace(err)
	}

	var httpClientGetter corehttp.HTTPClientGetter
	if err := getter.Get(config.HTTPClientName, &httpClientGetter); err != nil {
		return nil, errors.Trace(err)
	}

	httpClient, err := httpClientGetter.GetHTTPClient(ctx, corehttp.S3Purpose)
	if err != nil {
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
	return nil, errors.Errorf("in should be *s3caller.Client; got %T", in)
}

// NewS3Client returns a new S3 client based on the supplied dependencies.
func NewS3Client(endpoint string, client s3client.HTTPClient, creds s3client.Credentials, logger logger.Logger) (objectstore.Session, error) {
	return s3client.NewS3Client(endpoint, client, creds, logger)
}

// GetControllerConfigService is a helper function that gets a service from the
// manifold.
func GetControllerConfigService(getter dependency.Getter, name string) (ControllerConfigService, error) {
	return coredependency.GetDependencyByName(getter, name, func(factory services.ControllerObjectStoreServices) ControllerConfigService {
		return factory.ControllerConfig()
	})
}
