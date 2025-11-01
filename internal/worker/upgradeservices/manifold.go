// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradeservices

import (
	"context"

	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"

	"github.com/juju/juju/core/changestream"
	coredatabase "github.com/juju/juju/core/database"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/logger"
	domainservicefactory "github.com/juju/juju/domain/services"
	"github.com/juju/juju/internal/errors"
	internalservices "github.com/juju/juju/internal/services"
	"github.com/juju/juju/internal/worker/common"
)

// ManifoldConfig holds the information necessary to run a upgrade services
// factory worker in a dependency.Engine.
type ManifoldConfig struct {
	// ChangeStreamName is the name of the db getter dependency.
	ChangeStreamName string

	// Logger is the logger to use for the worker and upgrade services.
	Logger logger.Logger

	// NewProviderServices returns a new provider domain services for
	// the given model UUID.
	NewUpgradeServices UpgradeServicesFn

	// NewProviderServicesGetter returns a new provider domain services
	// getter, to select a provider domain services per model UUID.
	NewUpgradeServicesGetter UpgradeServicesGetterFn

	// NewWorker provides a func type for constructing a new upgrade services
	// worker.
	NewWorker func(Config) (worker.Worker, error)
}

// Manifold constructs a new dependency manifold for this worker and returns
// the information back to the caller.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{config.ChangeStreamName},
		Start:  config.start,
		Output: config.output,
	}
}

// NewUpgradeServices constructs and returns a
// [internalservices.UpgradeServices] for the caller. This is expected to be
// used with [ManifoldConfig.NewUpgradeServices].
func NewUpgradeServices(
	dbGetter changestream.WatchableDBGetter,
	logger logger.Logger,
) internalservices.UpgradeServices {
	return domainservicefactory.NewUpgradeServices(
		changestream.NewWatchableDBFactoryForNamespace(
			dbGetter.GetWatchableDB, coredatabase.ControllerNS,
		),
		logger,
	)
}

// NewUpgradeServicesGetter is responsible for constructing and providing a new
// [internalservices.UpgradeServicesGetter]. This is expected to be used with
// [ManifoldConfig.NewUpgradeServicesGetter]
func NewUpgradeServicesGetter(
	newUpgradeServices UpgradeServicesFn,
	dbGetter changestream.WatchableDBGetter,
	logger logger.Logger,
) internalservices.UpgradeServicesGetter {
	return &upgradeServicesGetter{
		newUpgradeServices: newUpgradeServices,
		dbGetter:           dbGetter,
		logger:             logger,
	}
}

// output provides the outwards dependencies of this worker back into the
// manifold. This func is capable of providing a [common.CleanupWorker] and a
// [internalservices.UpgradeServicesGetter].
//
// The following errors can be expected:
// - [coreerrors.NotValid] when the worker value is not of type [servicesWorker]
// - [coreerrors.NotSupport] when an output requested is not supported.
func (config ManifoldConfig) output(in worker.Worker, out any) error {
	if w, ok := in.(*common.CleanupWorker); ok {
		in = w.Worker
	}
	w, ok := in.(*servicesWorker)
	if !ok {
		return errors.Errorf(
			"expected input of type servicesWorker, got %T", in,
		).Add(coreerrors.NotValid)
	}

	switch out := out.(type) {
	case *internalservices.UpgradeServicesGetter:
		*out = w.servicesGetter
	default:
		return errors.Errorf("unsupported output type %T", out).Add(
			coreerrors.NotSupported,
		)
	}
	return nil
}

// start constructs and runs a new upgrade services worker based on the
// manifold configuration.
func (config ManifoldConfig) start(
	context context.Context,
	getter dependency.Getter,
) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Errorf("validating manifold config: %w", err)
	}

	var dbGetter changestream.WatchableDBGetter
	if err := getter.Get(config.ChangeStreamName, &dbGetter); err != nil {
		return nil, errors.Errorf("getting changestream db getter: %w", err)
	}

	return config.NewWorker(Config{
		DBGetter:                 dbGetter,
		Logger:                   config.Logger,
		NewUpgradeServicesGetter: config.NewUpgradeServicesGetter,
		NewUpgradeServices:       config.NewUpgradeServices,
	})
}

// Validate validates the manifold configuration ensuring the configuration
// values are in a state which be used to used to construct a new worker.
func (config ManifoldConfig) Validate() error {
	if config.ChangeStreamName == "" {
		return errors.New("empty ChangeStreamName not valid").Add(
			coreerrors.NotValid,
		)
	}
	if config.Logger == nil {
		return errors.New("nil Logger not valid").Add(coreerrors.NotValid)
	}
	if config.NewUpgradeServices == nil {
		return errors.New("nil NewUpgradeServices").Add(coreerrors.NotValid)
	}
	if config.NewUpgradeServicesGetter == nil {
		return errors.New("nil NewUpgradeServicesGetter").Add(coreerrors.NotValid)
	}
	if config.NewWorker == nil {
		return errors.New("nil NewWorker not valid").Add(coreerrors.NotValid)
	}
	return nil
}
