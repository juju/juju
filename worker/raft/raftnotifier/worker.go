// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package raftnotifier

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/catacomb"

	"github.com/juju/juju/core/raft/notifyproxy"
	"github.com/juju/juju/core/raftlease"
)

// Logger represents the logging methods called.
type Logger interface {
	Criticalf(message string, args ...interface{})
	Warningf(message string, args ...interface{})
	Errorf(message string, args ...interface{})
	Infof(message string, args ...interface{})
	Debugf(message string, args ...interface{})
	Tracef(message string, args ...interface{})
	Logf(level loggo.Level, message string, args ...interface{})
	IsTraceEnabled() bool
}

// Config is the configuration required for running a raft worker.
type Config struct {
	// Logger is the logger for this worker.
	Logger Logger

	// NotifyProxy is a proxy for notifications for the notify target.
	NotifyProxy notifyproxy.NotificationProxy

	// NotifyTarget is used to notify the changes from the raft operation
	// applications.
	NotifyTarget raftlease.NotifyTarget
}

// Validate validates the raft worker configuration.
func (config Config) Validate() error {
	if config.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	if config.NotifyProxy == nil {
		return errors.NotValidf("nil NotifyProxy")
	}
	if config.NotifyTarget == nil {
		return errors.NotValidf("nil NotifyTarget")
	}
	return nil
}

// NewWorker returns a new raft worker, with the given configuration.
func NewWorker(config Config) (worker.Worker, error) {
	return newWorker(config)
}

func newWorker(config Config) (*Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}
	w := &Worker{
		config: config,
	}
	if err := catacomb.Invoke(catacomb.Plan{
		Site: &w.catacomb,
		Work: func() error {
			return w.loop()
		},
	}); err != nil {
		return nil, errors.Trace(err)
	}
	return w, nil
}

// Worker is a worker that manages a raft.Raft instance.
type Worker struct {
	catacomb catacomb.Catacomb
	config   Config
}

// Kill is part of the worker.Worker interface.
func (w *Worker) Kill() {
	w.catacomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (w *Worker) Wait() error {
	return w.catacomb.Wait()
}

func (w *Worker) loop() error {
	for {
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()

		case changes := <-w.config.NotifyProxy.Notifications():
			// TODO: (stickupkid): We should push these as a batch through
			// the notify target.
			for _, change := range changes {
				switch change.Type() {
				case notifyproxy.Claimed:
					note := change.(notifyproxy.ClaimedNote)
					err := w.config.NotifyTarget.Claimed(note.Key, note.Holder)

					// We always want to sent the error response, so that the other
					// end of the proxy can be notified if there was an error or
					// not.
					note.ErrorResponse(err)

					// If there was an error, return out, so we get a fresh proxy
					// state.
					if err != nil {
						return errors.Trace(err)
					}

				case notifyproxy.Expiries:
					note := change.(notifyproxy.ExpiriesNote)
					err := w.config.NotifyTarget.Expiries(note.Expiries)

					// We always want to sent the error response, so that the other
					// end of the proxy can be notified if there was an error or
					// not.
					note.ErrorResponse(err)

					// If there was an error, return out, so we get a fresh proxy
					// state.
					if err != nil {
						return errors.Trace(err)
					}
				}
			}
		}
	}
}
