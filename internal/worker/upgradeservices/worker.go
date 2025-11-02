// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradeservices

import (
	"github.com/juju/worker/v4"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/core/changestream"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/internal/errors"
	internalservices "github.com/juju/juju/internal/services"
)

// Config is used to supply the required information for constructing a new
// upgrade services worker.
type Config struct {
	// DBGetter supplies WatchableDB implementations by namespace.
	DBGetter changestream.WatchableDBGetter

	// Logger is to be used by the upgrade services.
	Logger logger.Logger

	// NewUpgradeServicesGetter provides a new
	// [internalservices.UpgradeServicesGetter] for getting the upgrade
	// services.
	NewUpgradeServicesGetter UpgradeServicesGetterFn

	// NewUpgradeServices provides a way to construct a new
	// [internalservices.UpgradeServices].
	NewUpgradeServices UpgradeServicesFn
}

// servicesWorker is the long running worker in the controller supplying
// [internalservices.UpgradeServices] implementation.
type servicesWorker struct {
	tomb tomb.Tomb

	servicesGetter internalservices.UpgradeServicesGetter
}

// upgradeServicesGetter provides a worker based implementation of
// [internalservices.UpgradeServicesGetter].
type upgradeServicesGetter struct {
	dbGetter           changestream.WatchableDBGetter
	logger             logger.Logger
	newUpgradeServices UpgradeServicesFn
}

// UpgradeServicesGetterFn describes a func type capable of returning a new
// [internalservices.UpgradeServicesGetter]
type UpgradeServicesGetterFn func(
	UpgradeServicesFn, changestream.WatchableDBGetter, logger.Logger,
) internalservices.UpgradeServicesGetter

// UpgradeServicesFn describes a func type capable of returning a new
// [internalservices.UpgradeServices].
type UpgradeServicesFn func(
	changestream.WatchableDBGetter, logger.Logger,
) internalservices.UpgradeServices

// Kill marks this [servicesWorker] as dying. Implements the [worker.Worker]
// interface.
func (w *servicesWorker) Kill() {
	w.tomb.Kill(nil)
}

// NewWorker constructs a new worker responsible for handing out the upgrade
// services dependency into the model.
func NewWorker(config Config) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Errorf("validating worker config: %w", err)
	}

	w := &servicesWorker{
		servicesGetter: config.NewUpgradeServicesGetter(
			config.NewUpgradeServices,
			config.DBGetter,
			config.Logger,
		),
	}

	// services worker has no real work todo besides providing the upgrade
	// services dependency. We just hold a goroutine open until the tomb is
	// dying.
	w.tomb.Go(func() error {
		<-w.tomb.Dying()
		return w.tomb.Err()
	})
	return w, nil
}

// ServicesForController returns an [internalservices.UpgradeServices] for each
// request. This func implements the [internalservices.UpgradeServicesGetter]
// interface.
func (u *upgradeServicesGetter) ServicesForController() internalservices.UpgradeServices {
	return u.newUpgradeServices(
		u.dbGetter, u.logger,
	)
}

// ServicesGetter returns the [internalservices.UpgradeServicesGetter]
// associated with this worker.
func (s *servicesWorker) ServicesGetter() internalservices.UpgradeServicesGetter {
	return s.servicesGetter
}

// Validate checks the domain services configuration is valid for creating a new
// worker.
func (config Config) Validate() error {
	if config.DBGetter == nil {
		return errors.Errorf("nil DBGetter").Add(coreerrors.NotValid)
	}
	if config.Logger == nil {
		return errors.Errorf("nil Logger").Add(coreerrors.NotValid)
	}
	if config.NewUpgradeServices == nil {
		return errors.Errorf("nil NewUpgradeServices").Add(coreerrors.NotValid)
	}
	if config.NewUpgradeServicesGetter == nil {
		return errors.Errorf("nil NewUpgradeServicesGetter").Add(coreerrors.NotValid)
	}
	return nil
}

// Wait returns after the domain services worker has stopped. Implements the
// [worker.Worker] interface.
func (w *servicesWorker) Wait() error {
	return w.tomb.Wait()
}
