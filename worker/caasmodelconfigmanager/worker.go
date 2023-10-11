// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasmodelconfigmanager

import (
	"reflect"
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
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/docker"
	"github.com/juju/juju/docker/registry"
)

const (
	retryDuration   = 1 * time.Second
	refreshDuration = 30 * time.Second
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
	WatchControllerConfig() (watcher.NotifyWatcher, error)
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
	watcher, err := w.config.Facade.WatchControllerConfig()
	if err != nil {
		return errors.Trace(err)
	}
	err = w.catacomb.Add(watcher)
	if err != nil {
		return errors.Trace(err)
	}

	var (
		refresh         <-chan struct{}
		timeout         <-chan time.Time
		deadline        time.Time
		reg             registry.Registry
		lastRepoDetails docker.ImageRepoDetails
	)
	first := false
	signal := make(chan struct{})
	close(signal)
	defer func() {
		if reg != nil {
			_ = reg.Close()
		}
	}()

	for {
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()
		case <-watcher.Changes():
			controllerConfig, err := w.config.Facade.ControllerConfig()
			if err != nil {
				return errors.Trace(err)
			}
			repoDetails, err := docker.NewImageRepoDetails(controllerConfig.CAASImageRepo())
			if err != nil {
				return errors.Annotatef(err, "parsing %s", controller.CAASImageRepo)
			}
			if reflect.DeepEqual(repoDetails, lastRepoDetails) {
				continue
			}
			lastRepoDetails = repoDetails
			if !repoDetails.IsPrivate() {
				timeout = nil
				refresh = nil
				continue
			}
			if reg != nil {
				_ = reg.Close()
			}
			reg, err = w.registryFunc(repoDetails)
			if err != nil {
				return errors.Trace(err)
			}
			if err = reg.Ping(); err != nil {
				return errors.Trace(err)
			}
			first = true
			refresh = signal
		case <-timeout:
			timeout = nil
			if refresh == nil {
				refresh = signal
			}
		case <-refresh:
			refresh = nil
			next, err := w.ensureImageRepoSecret(reg, first)
			if err != nil {
				w.logger.Errorf("failed to update repository secret: %s", err.Error())
				next = retryDuration
			} else {
				first = false
			}
			if nextDeadline := w.clock.Now().Add(next); timeout == nil || nextDeadline.Before(deadline) {
				deadline = nextDeadline
				timeout = w.clock.After(next)
			}
		}
	}
}

func (w *manager) ensureImageRepoSecret(reg registry.Registry, force bool) (time.Duration, error) {
	shouldRefresh, nextRefresh := reg.ShouldRefreshAuth()
	if nextRefresh == time.Duration(0) {
		nextRefresh = refreshDuration
	}
	if !shouldRefresh && !force {
		return nextRefresh, nil
	}

	w.logger.Debugf("refreshing auth token for %q", w.name)
	if err := reg.RefreshAuth(); err != nil {
		return time.Duration(0), errors.Annotatef(err, "refreshing registry auth token for %q", w.name)
	}

	w.logger.Debugf("applying refreshed auth token for %q", w.name)
	err := w.config.Broker.EnsureImageRepoSecret(reg.ImageRepoDetails())
	if err != nil {
		return time.Duration(0), errors.Annotatef(err, "ensuring image repository secret for %q", w.name)
	}
	return nextRefresh, nil
}
