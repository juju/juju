// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controlsocket

import (
	"context"
	"path/filepath"

	"github.com/juju/errors"
	"github.com/juju/worker/v5"
	"github.com/juju/worker/v5/dependency"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/model"
	domainobjectstore "github.com/juju/juju/domain/objectstore"
	internalobjectstore "github.com/juju/juju/internal/objectstore"
	"github.com/juju/juju/internal/services"
	"github.com/juju/juju/internal/socketlistener"
	"github.com/juju/juju/internal/worker/common"
)

// ControllerObjectStoreService describes the subset of the object store service
// that is required by the controlsocket worker.
type ControllerObjectStoreService interface {
	// TransitionBackendToS3 sets the object store to use S3 with the provided
	// credentials. This is used to update the object store information when the
	// object store is set to use S3 as the backend.
	TransitionBackendToS3(ctx context.Context, credential domainobjectstore.S3Credentials) error
}

// GetControllerDomainServicesFunc is a function that retrieves the controller
// domain services from the dependency getter.
type GetControllerDomainServicesFunc func(dependency.Getter, string) (services.ControllerDomainServices, error)

// GetControllerObjectStoreServiceFunc is a function that retrieves the
// controller object store service from the dependency getter.
type GetControllerObjectStoreServiceFunc func(dependency.Getter, string) (ControllerObjectStoreService, error)

// GetObjectStoreServicesGetterFunc is a function that retrieves model object
// store services from the dependency getter.
type GetObjectStoreServicesGetterFunc func(dependency.Getter, string) (ObjectStoreServicesGetter, error)

// ManifoldConfig describes the dependencies required by the controlsocket worker.
type ManifoldConfig struct {
	DomainServicesName      string
	ObjectStoreServicesName string

	Logger            logger.Logger
	NewWorker         func(Config) (worker.Worker, error)
	NewSocketListener func(socketlistener.Config) (SocketListener, error)
	SocketName        string

	GetControllerDomainServices     GetControllerDomainServicesFunc
	GetControllerObjectStoreService GetControllerObjectStoreServiceFunc
	GetObjectStoreServicesGetter    GetObjectStoreServicesGetterFunc
	PrometheusRegisterer            prometheus.Registerer
	NewMetricsCollector             func() *Collector
}

// Manifold returns a Manifold that encapsulates the controlsocket worker.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.DomainServicesName,
			config.ObjectStoreServicesName,
		},
		Start: config.start,
	}
}

// Validate is called by start to check for bad configuration.
func (cfg ManifoldConfig) Validate() error {
	if cfg.DomainServicesName == "" {
		return errors.NotValidf("empty DomainServicesName")
	}
	if cfg.ObjectStoreServicesName == "" {
		return errors.NotValidf("empty ObjectStoreServicesName")
	}
	if cfg.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	if cfg.NewWorker == nil {
		return errors.NotValidf("nil NewWorker func")
	}
	if cfg.NewSocketListener == nil {
		return errors.NotValidf("nil NewSocketListener func")
	}
	if cfg.SocketName == "" {
		return errors.NotValidf("empty SocketName")
	}
	if cfg.GetControllerDomainServices == nil {
		return errors.NotValidf("nil GetControllerDomainServices func")
	}
	if cfg.GetControllerObjectStoreService == nil {
		return errors.NotValidf("nil GetControllerObjectStoreService func")
	}
	if cfg.GetObjectStoreServicesGetter == nil {
		return errors.NotValidf("nil GetObjectStoreServicesGetter func")
	}
	if cfg.PrometheusRegisterer == nil {
		return errors.NotValidf("nil PrometheusRegisterer")
	}
	if cfg.NewMetricsCollector == nil {
		return errors.NotValidf("nil NewMetricsCollector")
	}
	return nil
}

// start is a StartFunc for a Worker manifold.
func (cfg ManifoldConfig) start(ctx context.Context, getter dependency.Getter) (_ worker.Worker, err error) {
	if err = cfg.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	domainServices, err := cfg.GetControllerDomainServices(getter, cfg.DomainServicesName)
	if err != nil {
		return nil, errors.Trace(err)
	}

	controllerObjectStoreService, err := cfg.GetControllerObjectStoreService(getter, cfg.ObjectStoreServicesName)
	if err != nil {
		return nil, errors.Trace(err)
	}

	controllerMetadataService, ok := controllerObjectStoreService.(MetadataService)
	if !ok {
		return nil, errors.NotValidf("controller object store service does not support metadata listing")
	}

	objectStoreServicesGetter, err := cfg.GetObjectStoreServicesGetter(getter, cfg.ObjectStoreServicesName)
	if err != nil {
		return nil, errors.Trace(err)
	}

	controllerSerivce := domainServices.Controller()
	controllerModelUUID, err := controllerSerivce.ControllerModelUUID(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}

	metricsCollector := cfg.NewMetricsCollector()
	if err := cfg.PrometheusRegisterer.Register(metricsCollector); err != nil {
		return nil, errors.Trace(err)
	}

	preflightValidator, err := NewDrainPreflightValidator(DrainPreflightValidatorConfig{
		ControllerService:         controllerSerivce,
		ControllerMetadataService: controllerMetadataService,
		ObjectStoreServicesGetter: objectStoreServicesGetter,
		NewHashFileSystemAccessor: NewHashFileStoreAccessor,
		SelectFileHash:            internalobjectstore.SelectFileHash,
		RootDir:                   filepath.Dir(cfg.SocketName),
		Logger:                    cfg.Logger,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}

	var w worker.Worker
	w, err = cfg.NewWorker(Config{
		AccessService:           domainServices.Access(),
		TracingService:          domainServices.Tracing(),
		LoggingService:          domainServices.Logging(),
		ObjectStoreService:      controllerObjectStoreService,
		DrainPreflightValidator: preflightValidator,
		Logger:                  cfg.Logger,
		SocketName:              cfg.SocketName,
		NewSocketListener:       cfg.NewSocketListener,
		ControllerModelUUID:     controllerModelUUID,
		MetricsCollector:        metricsCollector,
	})
	if err != nil {
		cfg.PrometheusRegisterer.Unregister(metricsCollector)
		return nil, errors.Trace(err)
	}
	return common.NewCleanupWorker(w, func() {
		cfg.PrometheusRegisterer.Unregister(metricsCollector)
	}), nil
}

// SocketListener describes a worker that listens on a unix socket.
type SocketListener interface {
	worker.Worker
}

// NewSocketListener is a function that creates a new socket listener.
func NewSocketListener(config socketlistener.Config) (SocketListener, error) {
	return socketlistener.NewSocketListener(config)
}

// GetControllerDomainServices retrieves the controller domain services from the
// dependency getter.
func GetControllerDomainServices(getter dependency.Getter, name string) (services.ControllerDomainServices, error) {
	var domainServices services.ControllerDomainServices
	if err := getter.Get(name, &domainServices); err != nil {
		return nil, errors.Trace(err)
	}
	return domainServices, nil
}

// GetControllerObjectStoreService retrieves the controller object store service
// from the dependency getter.
func GetControllerObjectStoreService(getter dependency.Getter, name string) (ControllerObjectStoreService, error) {
	var services services.ControllerObjectStoreServices
	if err := getter.Get(name, &services); err != nil {
		return nil, errors.Trace(err)
	}
	return services.AgentObjectStore(), nil
}

// GetObjectStoreServicesGetter retrieves model object store services from the
// dependency getter.
func GetObjectStoreServicesGetter(getter dependency.Getter, name string) (ObjectStoreServicesGetter, error) {
	var servicesGetter services.ObjectStoreServicesGetter
	if err := getter.Get(name, &servicesGetter); err != nil {
		return nil, errors.Trace(err)
	}
	return modelMetadataServiceGetter{
		servicesGetter: servicesGetter,
	}, nil
}

// NewHashFileStoreAccessor creates a hash accessor rooted at the supplied
// namespace and root directory.
func NewHashFileStoreAccessor(
	namespace, rootDir string, logger logger.Logger,
) HashFileSystemAccessor {
	return internalobjectstore.NewHashFileStore(namespace, rootDir, logger)
}

type modelMetadataServiceGetter struct {
	servicesGetter services.ObjectStoreServicesGetter
}

// ObjectStoreForModel returns metadata operations for the supplied model.
func (s modelMetadataServiceGetter) ObjectStoreForModel(modelUUID model.UUID) MetadataService {
	return s.servicesGetter.ServicesForModel(modelUUID).ObjectStore()
}
