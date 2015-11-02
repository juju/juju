// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmrevision

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/utils/clock"
	"launchpad.net/tomb"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/charmrevisionupdater"
	"github.com/juju/juju/worker"
)

// Facade exposes the capabilities required by the worker.
type Facade interface {

	// UpdateLatestRevisions causes the environment to be scanned, the charm
	// store to be interrogated, and model representations of updated charms
	// to be stored in the environment.
	//
	// That is sufficiently complex that the logic should be implemented by
	// the worker, not directly on the apiserver; as this functionality needs
	// to change/mature, please migrate responsibilities down to the worker
	// and grow this interface to match.
	UpdateLatestRevisions() error
}

// NewFacade returns a Facade backed by the supplied APICaller.
func NewFacade(apiCaller base.APICaller) (Facade, error) {
	return charmrevisionupdater.NewState(apiCaller), nil
}

// Config defines the operation of a charm revision updater worker.
type Config struct {

	// Facade is the worker's view of the controller.
	Facade Facade

	// Clock is the worker's view of time.
	Clock clock.Clock

	// Period is the time between charm revision updates.
	Period time.Duration
}

// Validate returns an error if the configuration cannot be expected
// to start a functional worker.
func (config Config) Validate() error {
	if config.Facade == nil {
		return errors.NotValidf("nil Facade")
	}
	if config.Clock == nil {
		return errors.NotValidf("nil Clock")
	}
	if config.Period <= 0 {
		return errors.NotValidf("non-positive Period")
	}
	return nil
}

// NewWorker returns a worker that calls UpdateLatestRevisions on the
// configured Facade, once when started and subsequently every Period.
func NewWorker(config Config) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}
	w := &revisionUpdateWorker{
		config: config,
	}
	go func() {
		defer w.tomb.Done()
		w.tomb.Kill(w.loop())
	}()
	return w, nil
}

type revisionUpdateWorker struct {
	tomb   tomb.Tomb
	config Config
}

func (ruw *revisionUpdateWorker) loop() error {
	var delay time.Duration
	for {
		select {
		case <-ruw.tomb.Dying():
			return tomb.ErrDying
		case <-ruw.config.Clock.After(delay):
			err := ruw.config.Facade.UpdateLatestRevisions()
			if err != nil {
				return errors.Trace(err)
			}
		}
		delay = ruw.config.Period
	}
}

// Kill is part of the worker.Worker interface.
func (ruw *revisionUpdateWorker) Kill() {
	ruw.tomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (ruw *revisionUpdateWorker) Wait() error {
	return ruw.tomb.Wait()
}
