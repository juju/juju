// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasmodelconfigmanager

import (
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/catacomb"

	"github.com/juju/juju/api/base"
	api "github.com/juju/juju/api/controller/caasmodelconfigmanager"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/internal/docker"
	"github.com/juju/juju/internal/docker/registry"
)

// Logger represents the methods used by the worker to log details.
type Logger interface {
	Debugf(string, ...interface{})
	Infof(string, ...interface{})
	Errorf(string, ...interface{})
	Warningf(string, ...interface{})
	Tracef(string, ...interface{})

	Child(string) loggo.Logger
}

//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/facade_mock.go github.com/juju/juju/worker/caasmodelconfigmanager Facade
type Facade interface {
	ControllerConfig() (controller.Config, error)
}

//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/broker_mock.go github.com/juju/juju/worker/caasmodelconfigmanager CAASBroker
type CAASBroker interface {
	EnsureImageRepoSecret(docker.ImageRepoDetails) error
}

// Config holds the configuration and dependencies for a worker.
type Config struct {
	ModelTag names.ModelTag

	Facade       Facade
	Broker       CAASBroker
	Logger       Logger
	Clock        clock.Clock
	RegistryFunc func(docker.ImageRepoDetails) (registry.Registry, error)
}

// Validate returns an error if the config cannot be expected
// to drive a functional worker.
func (config Config) Validate() error {
	if config.ModelTag == (names.ModelTag{}) {
		return errors.NotValidf("ModelTag is missing")
	}
	if config.Facade == nil {
		return errors.NotValidf("Facade is missing")
	}
	if config.Broker == nil {
		return errors.NotValidf("Broker is missing")
	}
	if config.Logger == nil {
		return errors.NotValidf("Logger is missing")
	}
	if config.Clock == nil {
		return errors.NotValidf("Clock is missing")
	}
	if config.RegistryFunc == nil {
		return errors.NotValidf("RegistryFunc is missing")
	}
	return nil
}

type manager struct {
	catacomb catacomb.Catacomb

	name   string
	config Config
	logger Logger
	clock  clock.Clock

	registryFunc func(docker.ImageRepoDetails) (registry.Registry, error)
	reg          registry.Registry

	nextTickDuration *time.Duration
	ticker           clock.Timer
}

// NewFacade returns a facade for caasapplicationprovisioner worker to use.
func NewFacade(caller base.APICaller) (Facade, error) {
	return api.NewClient(caller)
}

// NewWorker returns a worker that unlocks the model upgrade gate.
func NewWorker(config Config) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}
	w := &manager{
		name:         config.ModelTag.Id(),
		config:       config,
		logger:       config.Logger,
		clock:        config.Clock,
		registryFunc: config.RegistryFunc,
	}
	err := catacomb.Invoke(catacomb.Plan{
		Site: &w.catacomb,
		Work: w.loop,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return w, nil
}

// Kill is part of the worker.Worker interface.
func (w *manager) Kill() {
	w.catacomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (w *manager) Wait() error {
	return w.catacomb.Wait()
}

func (w *manager) loop() (err error) {
	defer func() {
		if w.ticker != nil && !w.ticker.Stop() {
			select {
			case <-w.ticker.Chan():
			default:
			}
		}
	}()
	controllerConfig, err := w.config.Facade.ControllerConfig()
	if err != nil {
		return errors.Trace(err)
	}
	repoDetails := controllerConfig.CAASImageRepo()
	if !repoDetails.IsPrivate() {
		// No ops for public registry config.
		return nil
	}
	w.reg, err = w.registryFunc(repoDetails)
	if err != nil {
		return errors.Trace(err)
	}
	if err = w.reg.Ping(); err != nil {
		return errors.Trace(err)
	}
	if err := w.ensureImageRepoSecret(true); err != nil {
		return errors.Trace(err)
	}

	for {
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()
		case <-w.getTickerChan():
			if err := w.ensureImageRepoSecret(false); err != nil {
				return errors.Trace(err)
			}
		}
	}
}

func (w *manager) getTickerChan() <-chan time.Time {
	d := w.getTickerDuration()
	if w.ticker == nil {
		w.ticker = w.clock.NewTimer(d)
	} else {
		if !w.ticker.Stop() {
			select {
			case <-w.ticker.Chan():
			default:
			}
		}
		w.ticker.Reset(d)
	}
	return w.ticker.Chan()
}

func (w *manager) getTickerDuration() time.Duration {
	if w.nextTickDuration != nil {
		return *w.nextTickDuration
	}
	return 30 * time.Second
}

func (w *manager) ensureImageRepoSecret(isFirstCall bool) error {
	var shouldRefresh bool
	if shouldRefresh, w.nextTickDuration = w.reg.ShouldRefreshAuth(); !shouldRefresh && !isFirstCall {
		return nil
	}
	if err := w.reg.RefreshAuth(); err != nil {
		return errors.Annotatef(err, "refreshing registry auth token for %q", w.name)
	}
	w.logger.Debugf("auth token for %q has been refreshed, applying to the secret now", w.name)
	err := w.config.Broker.EnsureImageRepoSecret(w.reg.ImageRepoDetails())
	return errors.Annotatef(err, "ensuring image repository secret for %q", w.name)
}
