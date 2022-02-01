// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmrevision

import (
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/retry"
	"github.com/juju/worker/v3"
	"gopkg.in/tomb.v2"
)

// logger is here to stop the desire of creating a package level logger.
// Don't do this, instead pass one in.
type logger interface{}

var _ logger = struct{}{}

// RevisionUpdater exposes the "single" capability required by the worker.
// As the worker gains more responsibilities, it will likely need more; see
// storageprovisioner for a helpful model to grow towards.
type RevisionUpdater interface {

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

// Config defines the operation of a charm revision updater worker.
type Config struct {

	// RevisionUpdater is the worker's view of the controller.
	RevisionUpdater RevisionUpdater

	// Clock is the worker's view of time.
	Clock clock.Clock

	// Period is the time between charm revision updates.
	Period time.Duration

	// Logger is the logger used for debug logging in this worker.
	Logger Logger
}

// Logger is a debug-only logger interface.
type Logger interface {
	Debugf(message string, args ...interface{})
}

// Validate returns an error if the configuration cannot be expected
// to start a functional worker.
func (config Config) Validate() error {
	if config.RevisionUpdater == nil {
		return errors.NotValidf("nil RevisionUpdater")
	}
	if config.Clock == nil {
		return errors.NotValidf("nil Clock")
	}
	if config.Period <= 0 {
		return errors.NotValidf("non-positive Period")
	}
	if config.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	return nil
}

// NewWorker returns a worker that calls UpdateLatestRevisions on the
// configured RevisionUpdater, once when started and subsequently every
// Period.
func NewWorker(config Config) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}
	w := &revisionUpdateWorker{
		config: config,
	}
	w.config.Logger.Debugf("worker created with period %v", w.config.Period)
	w.tomb.Go(w.loop)
	return w, nil
}

type revisionUpdateWorker struct {
	tomb   tomb.Tomb
	config Config
}

func (ruw *revisionUpdateWorker) loop() error {
	for {
		select {
		case <-ruw.tomb.Dying():
			return tomb.ErrDying

		// TODO (stickupkid): Instead of applying a large jitter, we should
		// instead attempt to claim a lease to for the required period. Release
		// the lease on the termination of the worker. Other HA nodes can
		// update then claim the lease and run the checks.
		case <-ruw.config.Clock.After(jitter(ruw.config.Period)):
			ruw.config.Logger.Debugf("%v elapsed, performing work", ruw.config.Period)
			err := ruw.config.RevisionUpdater.UpdateLatestRevisions()
			if err != nil {
				return errors.Trace(err)
			}
		}
	}
}

func jitter(period time.Duration) time.Duration {
	return retry.ExpBackoff(period, period*2, 2, true)(0, 1)
}

// Kill is part of the worker.Worker interface.
func (ruw *revisionUpdateWorker) Kill() {
	ruw.tomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (ruw *revisionUpdateWorker) Wait() error {
	return ruw.tomb.Wait()
}
