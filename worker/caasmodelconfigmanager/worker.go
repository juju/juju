// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasmodelconfigmanager

import (
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/dependency"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/controller"
	"github.com/juju/juju/docker"
)

const (
	// defaultDuration is the default duration between refreshes.
	defaultDuration = time.Second * 30
)

// RegistryFunc is a function that returns a registry for the given image
// repository details.
type RegistryFunc func(docker.ImageRepoDetails) (Registry, error)

// ImageRepoFunc is a function that returns an image repo for the given path.
type ImageRepoFunc func(string) (ImageRepo, error)

// ControllerConfigService represents the methods used by the worker to interact
// with the controller config.
type ControllerConfigService interface {
	ControllerConfig() (controller.Config, error)
}

// CAASBroker represents the methods used by the worker to interact with the
// CAAS broker.
type CAASBroker interface {
	EnsureImageRepoSecret(docker.ImageRepoDetails) error
}

// Registry represents the methods used by the worker to interact with the
// registry.
type Registry interface {
	ImageRepoDetails() docker.ImageRepoDetails
	ShouldRefreshAuth() (bool, *time.Duration)
	RefreshAuth() error
	Ping() error
	Close() error
}

// ImageRepo represents the methods used by the worker to interact with the
// image repo.
type ImageRepo interface {
	// RequestDetails returns the details of the image repo.
	RequestDetails() (docker.ImageRepoDetails, error)
}

// Config holds the configuration and dependencies for a worker.
type Config struct {
	ModelTag names.ModelTag

	ControllerConfigService ControllerConfigService
	Broker                  CAASBroker
	Logger                  Logger
	Clock                   clock.Clock
	RegistryFunc            RegistryFunc
	ImageRepoFunc           ImageRepoFunc
}

// Validate returns an error if the config cannot be expected
// to drive a functional worker.
func (config Config) Validate() error {
	if config.ModelTag == (names.ModelTag{}) {
		return errors.NotValidf("ModelTag is missing")
	}
	if config.ControllerConfigService == nil {
		return errors.NotValidf("ControllerConfigService is missing")
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
	if config.ImageRepoFunc == nil {
		return errors.NotValidf("ImageRepoFunc is missing")
	}
	return nil
}

type manager struct {
	tomb tomb.Tomb

	name   string
	config Config
	logger Logger
	clock  clock.Clock

	registryFn  RegistryFunc
	imageRepoFn ImageRepoFunc

	nextTickDuration *time.Duration
	ticker           clock.Timer
}

// NewWorker returns a worker that unlocks the model upgrade gate.
func NewWorker(config Config) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}
	w := &manager{
		name:        config.ModelTag.Id(),
		config:      config,
		logger:      config.Logger,
		clock:       config.Clock,
		registryFn:  config.RegistryFunc,
		imageRepoFn: config.ImageRepoFunc,
	}
	w.tomb.Go(w.loop)
	return w, nil
}

// Kill is part of the worker.Worker interface.
func (w *manager) Kill() {
	w.tomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (w *manager) Wait() error {
	return w.tomb.Wait()
}

func (w *manager) loop() (err error) {
	controllerConfig, err := w.config.ControllerConfigService.ControllerConfig()
	if err != nil {
		return errors.Annotatef(err, "cannot get controller config")
	}

	// This is no CAAS image repo configured, this is a read-only controller
	// config value, so we can uninstall ourselves.
	path := controllerConfig.CAASImageRepo()
	if path == "" {
		return dependency.ErrUninstall
	}

	repo, err := w.imageRepoFn(path)
	if err != nil {
		return errors.Annotatef(err, "cannot create image repo")
	}

	details, err := repo.RequestDetails()
	if err != nil {
		return errors.Annotatef(err, "cannot get image repo details for %q", path)
	}
	// If the details are empty or the details are public, then return out
	// early. These values are read-only, so we can uninstall ourselves.
	if details.Empty() || !details.IsPrivate() {
		return dependency.ErrUninstall
	}

	reg, err := w.registryFn(details)
	if err != nil {
		return errors.Trace(err)
	}
	if err = reg.Ping(); err != nil {
		return errors.Trace(err)
	}
	if err := w.ensureImageRepoSecret(reg, true); err != nil {
		return errors.Trace(err)
	}

	defer drainTicker(w.ticker)
	for {
		select {
		case <-w.tomb.Dying():
			return tomb.ErrDying
		case <-w.getTickerChan():
			if err := w.ensureImageRepoSecret(reg, false); err != nil {
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
		drainTicker(w.ticker)
		w.ticker.Reset(d)
	}
	return w.ticker.Chan()
}

func (w *manager) getTickerDuration() time.Duration {
	if w.nextTickDuration != nil {
		return *w.nextTickDuration
	}
	return defaultDuration
}

func (w *manager) ensureImageRepoSecret(reg Registry, isFirstCall bool) error {
	var shouldRefresh bool
	if shouldRefresh, w.nextTickDuration = reg.ShouldRefreshAuth(); !shouldRefresh && !isFirstCall {
		return nil
	}
	if err := reg.RefreshAuth(); err != nil {
		return errors.Annotatef(err, "refreshing registry auth token for %q", w.name)
	}
	w.logger.Debugf("auth token for %q has been refreshed, applying to the secret now", w.name)
	err := w.config.Broker.EnsureImageRepoSecret(reg.ImageRepoDetails())
	return errors.Annotatef(err, "ensuring image repository secret for %q", w.name)
}

func drainTicker(t clock.Timer) {
	if t == nil {
		return
	}
	if t.Stop() {
		return
	}

	// Drain the channel if the timer was not stopped.
	select {
	case <-t.Chan():
	default:
	}
}
